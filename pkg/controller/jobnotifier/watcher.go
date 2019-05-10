package jobnotifier

import (
	"fmt"
	"reflect"
	"sync"

	jsnv1beta1 "github.com/bgpat/job-slack-notifier/pkg/apis/jsn/v1beta1"
	"github.com/bgpat/job-slack-notifier/pkg/notifier"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type watcher struct {
	watch.Interface
	notifier *jsnv1beta1.JobNotifier
	stopCh   chan struct{}
}

var (
	watchers   = map[types.UID]*watcher{}
	watchersMu sync.RWMutex

	k8sClient *kubernetes.Clientset

	msgTimestampKey = jsnv1beta1.SchemeGroupVersion.Group + "/message-timestamp"
)

func watchJob(notifier *jsnv1beta1.JobNotifier) error {
	watchersMu.RLock()
	w, exist := watchers[notifier.UID]
	watchersMu.RUnlock()
	if exist {
		if reflect.DeepEqual(w.notifier, notifier) {
			log.Info(
				"Not changed",
				"namespace", notifier.Namespace,
				"name", notifier.Name,
			)
			return nil
		}
		w.stop()
	}
	w = &watcher{
		notifier: notifier,
		stopCh:   make(chan struct{}),
	}
	watchersMu.Lock()
	watchers[notifier.UID] = w
	watchersMu.Unlock()
	return w.start()
}

func (w *watcher) start() error {
	selector := metav1.FormatLabelSelector(w.notifier.Spec.Selector)
	i, err := k8sClient.BatchV1().Jobs(w.notifier.Namespace).Watch(metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		log.Error(
			err, "Cloud not get Job",
			"namespace", w.notifier.Namespace,
			"notifier", w.notifier.Name,
			"selector", selector,
		)
		return err
	}
	w.Interface = i
	log.Info(
		"start watcher",
		"namespace", w.notifier.Namespace,
		"notifier", w.notifier.Name,
		"selector", selector,
	)
	go func() {
		for {
			select {
			case <-w.stopCh:
				i.Stop()
				return
			case ev := <-i.ResultChan():
				w.process(ev)
			}
		}
	}()
	return nil
}

func (w *watcher) stop() {
	log.Info(
		"stop watcher",
		"namespace", w.notifier.Namespace,
		"notifier", w.notifier.Name,
		"selector", metav1.FormatLabelSelector(w.notifier.Spec.Selector),
	)
	close(w.stopCh)
	watchersMu.Lock()
	delete(watchers, w.notifier.UID)
	watchersMu.Unlock()
}

func (w *watcher) process(ev watch.Event) error {
	job, ok := ev.Object.(*batchv1.Job)
	if !ok {
		return fmt.Errorf("Could not cast Job from %T", ev.Object)
	}
	selector := metav1.FormatLabelSelector(job.Spec.Selector)
	pods, err := k8sClient.CoreV1().Pods(job.Namespace).List(metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		log.Error(
			err, "Cloud not get Pods",
			"namespace", job.Namespace,
			"name", job.Name,
			"selector", selector,
		)
		return err
	}
	ts := job.GetAnnotations()[msgTimestampKey]
	// TODO: get channel ID from CRD
	newTs, unlock, err := notifier.Post("CJKV1EJ56", ts, ev.Type, job, pods.Items)
	if unlock != nil {
		defer unlock()
	}
	log.Info(
		"process",
		"type", ev.Type,
		"status", job.Status,
		"ts", newTs,
	)
	if err != nil {
		return err
	}
	if newTs != "" && newTs != ts {
		metav1.SetMetaDataAnnotation(&job.ObjectMeta, msgTimestampKey, newTs)
		_, err := k8sClient.BatchV1().Jobs(w.notifier.Namespace).Update(job)
		if err != nil {
			log.Error(
				err, "Cloud not update Job",
				"namespace", w.notifier.Namespace,
				"notifier", w.notifier.Name,
			)
			return err
		}
	}
	return err
}
