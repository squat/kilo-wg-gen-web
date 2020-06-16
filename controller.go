package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/squat/kilo/pkg/k8s/apis/kilo/v1alpha1"
	kiloclient "github.com/squat/kilo/pkg/k8s/clientset/versioned"
	v1alpha1informers "github.com/squat/kilo/pkg/k8s/informers/kilo/v1alpha1"
	v1alpha1listers "github.com/squat/kilo/pkg/k8s/listers/kilo/v1alpha1"
	"gitlab.127-0-0-1.fr/vx3r/wg-gen-web/model"
	"gopkg.in/fsnotify.v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

type controller struct {
	client   kiloclient.Interface
	dir      string
	informer cache.SharedIndexInformer
	lister   v1alpha1listers.PeerLister
	logger   log.Logger
	queue    workqueue.RateLimitingInterface
	server   *model.Server

	mu    sync.Mutex
	peers map[string]*v1alpha1.Peer

	peersG            prometheus.Gauge
	reconcileAttempts prometheus.Counter
	reconcileErrors   prometheus.Counter
}

func newController(dir string, kc kiloclient.Interface, logger log.Logger, reg prometheus.Registerer, server *model.Server) *controller {
	selector := labels.Set{managedByLabelKey: managedByLabelValue}.AsSelector()
	pi := v1alpha1informers.NewFilteredPeerInformer(kc, 5*time.Minute, nil, func(options *metav1.ListOptions) { options.LabelSelector = selector.String() })
	if logger == nil {
		logger = log.NewNopLogger()
	}
	c := controller{
		dir:      dir,
		client:   kc,
		informer: pi,
		lister:   v1alpha1listers.NewPeerLister(pi.GetIndexer()),
		logger:   logger,
		queue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "service-reflector"),
		server:   server,

		peers: make(map[string]*v1alpha1.Peer),

		peersG: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "kilo_wg_gen_web_peers",
			Help: "Number of peers loaded from disk",
		}),
		reconcileAttempts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kilo_wg_gen_web_reconcile_attempts_total",
			Help: "Number of attempts to reconcile peers",
		}),
		reconcileErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kilo_wg_gen_web_reconcile_errors_total",
			Help: "Number of errors that occurred while reconciling peers",
		}),
	}

	if reg != nil {
		reg.MustRegister(c.peersG, c.reconcileAttempts, c.reconcileErrors)
	}

	return &c
}

func (c *controller) run(stop <-chan struct{}) error {
	defer c.queue.ShutDown()

	go c.informer.Run(stop)
	if ok := cache.WaitForCacheSync(stop, func() bool {
		return c.informer.HasSynced()
	}); !ok {
		return errors.New("sync peer cache")
	}

	go c.worker()

	if err := c.refresh(); err != nil {
		return fmt.Errorf("initial refresh: %w", err)
	}

	// Add handlers after initial refresh and sync.
	c.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handle,
		UpdateFunc: func(_, obj interface{}) { c.handle(obj) },
		DeleteFunc: c.handle,
	})

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("setting up watcher: %w", err)
	}
	if err := watcher.Add(c.dir); err != nil {
		return fmt.Errorf("watch directory %q: %w", c.dir, err)
	}
	for {
		select {
		case event := <-watcher.Events:
			// fsnotify sometimes sends a bunch of events without name or operation.
			// It's unclear what they are and why they are sent - filter them out.
			if len(event.Name) == 0 {
				break
			}

			level.Debug(c.logger).Log("msg", "refreshing peers from disk")
			if err := c.refresh(); err != nil {
				level.Error(c.logger).Log("err", fmt.Sprintf("refresh configuration directory: %v", err))
			}
		case <-stop:
			return nil
		}
	}
}

func (c *controller) refresh() error {
	files, err := ioutil.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("read configuration directory %q: %w", c.dir, err)
	}

	var n float64
	peers := make(map[string]*v1alpha1.Peer)
	for _, f := range files {
		if _, err := uuid.Parse(f.Name()); err != nil {
			// This file is not a peer configuration file; skip it.
			continue
		}
		j, err := ioutil.ReadFile(path.Join(c.dir, f.Name()))
		if err != nil {
			return fmt.Errorf("read configuration file %q: %w", f.Name(), err)
		}
		cfg := &model.Client{}
		if err := json.Unmarshal(j, cfg); err != nil {
			return fmt.Errorf("unmarshal configuration as JSON: %w", err)
		}
		if !cfg.Enable {
			continue
		}
		peers[cfg.Id] = translate(cfg, c.server.PersistentKeepalive)
		n++
	}

	c.peersG.Set(n)
	c.mu.Lock()
	// Add all peers that have been deleted from disk to the queue.
	for peer := range c.peers {
		if _, ok := peers[peer]; !ok {
			c.queue.Add(peer)
		}
	}
	c.peers = peers
	// Add all new peers to the queue, as the may not be in the API.
	for peer := range peers {
		c.queue.Add(peer)
	}
	c.mu.Unlock()
	return nil
}

func (c *controller) worker() {
	level.Debug(c.logger).Log("msg", "starting worker")
	for c.processNextWorkItem() {
	}
	level.Debug(c.logger).Log("msg", "stopping worker")
}

func (c *controller) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	level.Debug(c.logger).Log("msg", "processing queue item", "key", key)
	defer c.queue.Done(key)

	c.reconcileAttempts.Inc()
	err := c.sync(key.(string))
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	c.reconcileErrors.Inc()
	runtime.HandleError(fmt.Errorf("sync %q failed: %w", key, err))
	c.queue.AddRateLimited(key)

	return true
}

func (c *controller) sync(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If a peer from the API is not in the listed files, delete it from the API.
	if _, ok := c.peers[name]; !ok {
		level.Info(c.logger).Log("msg", fmt.Sprintf("deleteing peer %s from API", name))
		if err := c.client.KiloV1alpha1().Peers().Delete(name, &metav1.DeleteOptions{}); err != nil && !kerrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	peer, err := c.lister.Get(name)
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}
	// If a peer on disk is not in the API, create it.
	if peer == nil || kerrors.IsNotFound(err) {
		level.Info(c.logger).Log("msg", fmt.Sprintf("creating peer %s in API", name))
		if _, err = c.client.KiloV1alpha1().Peers().Create(c.peers[name]); err != nil && !kerrors.IsAlreadyExists(err) {
			return err
		}
		return nil
	}

	// If the peers are equal, then our work is done.
	if len(peer.Spec.AllowedIPs) == len(c.peers[name].Spec.AllowedIPs) && strings.Join(peer.Spec.AllowedIPs, ",") == strings.Join(c.peers[name].Spec.AllowedIPs, ",") && len(peer.Spec.PublicKey) == len(c.peers[name].Spec.PublicKey) && peer.Spec.PersistentKeepalive == c.peers[name].Spec.PersistentKeepalive {
		return nil
	}

	// If the peers are not equal, then update the API.
	level.Info(c.logger).Log("msg", fmt.Sprintf("peer %s has changed; updating", name))
	_, err = c.client.KiloV1alpha1().Peers().Update(c.peers[name])
	return err
}

func (c *controller) handle(obj interface{}) {
	c.queue.Add(obj.(*v1alpha1.Peer).Name)
}

func translate(cfg *model.Client, defaultPersistentKeepalive int) *v1alpha1.Peer {
	var pka int
	if !cfg.IgnorePersistentKeepalive {
		pka = defaultPersistentKeepalive
	}

	return &v1alpha1.Peer{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfg.Id,
			Labels: map[string]string{
				managedByLabelKey: managedByLabelValue,
			},
		},
		Spec: v1alpha1.PeerSpec{
			AllowedIPs:          cfg.Address,
			PersistentKeepalive: pka,
			PresharedKey:        cfg.PresharedKey,
			PublicKey:           cfg.PublicKey,
		},
	}
}
