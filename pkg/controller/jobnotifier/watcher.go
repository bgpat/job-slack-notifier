package jobnotifier

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"sync"

	jsnv1beta1 "github.com/bgpat/job-slack-notifier/pkg/apis/jsn/v1beta1"
	"github.com/bgpat/job-slack-notifier/pkg/notifier"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
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
	podLogMu   = sync.Map{}

	k8sClient *kubernetes.Clientset

	msgTimestampKey    = jsnv1beta1.SchemeGroupVersion.Group + "/message-ts-"
	podLogTimestampKey = jsnv1beta1.SchemeGroupVersion.Group + "/log-ts-"

	// TODO: get channel ID from CRD
	channel = "CJKV1EJ56"
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
	ts := job.GetAnnotations()[msgTimestampKey+channel]
	newTS, unlock, err := notifier.Send(channel, ts, ev.Type, job, pods.Items)
	if unlock != nil {
		defer unlock()
	}
	log.Info(
		"process",
		"type", ev.Type,
		"status", job.Status,
		"ts", newTS,
	)
	if err != nil {
		return err
	}
	if newTS != "" && newTS != ts {
		metav1.SetMetaDataAnnotation(&job.ObjectMeta, msgTimestampKey+channel, newTS)
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
	for _, pod := range pods.Items {
		if len(pod.Status.ContainerStatuses) <= 0 {
			continue
		}
		terminated := true
		containers := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
		containers = append(containers, pod.Status.InitContainerStatuses...)
		containers = append(containers, pod.Status.ContainerStatuses...)
		for _, ct := range containers {
			if ct.State.Terminated == nil {
				terminated = false
				break
			}
		}
		if terminated {
			go w.sendPodLog(newTS, pod)
		}
	}
	return err
}

func (w *watcher) sendPodLog(ts string, pod corev1.Pod) {
	v, _ := podLogMu.LoadOrStore(pod.UID, &sync.RWMutex{})
	mu := v.(*sync.RWMutex)
	mu.RLock()
	skip := metav1.HasAnnotation(pod.ObjectMeta, podLogTimestampKey+channel)
	mu.RUnlock()
	if skip {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	containers := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	containers = append(containers, pod.Status.InitContainerStatuses...)
	containers = append(containers, pod.Status.ContainerStatuses...)
	logs := map[string]string{}
	for _, ct := range containers {
		req := k8sClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: ct.Name,
			TailLines: func(i int64) *int64 { return &i }(100),
		})
		stream, err := req.Stream()
		if err != nil {
			log.Error(
				err, "Could not get log stream",
				"namespace", pod.Namespace,
				"pod", pod.Name,
				"container", ct.Name,
			)
			return
		}
		l, err := ioutil.ReadAll(stream)
		if err != nil {
			log.Error(
				err, "Could not read log stream",
				"namespace", pod.Namespace,
				"pod", pod.Name,
				"container", ct.Name,
			)
			return
		}
		logs[ct.Name] = string(l)
	}
	newTS, err := notifier.SendLogs(channel, ts, pod, logs)
	if err != nil {
		log.Error(
			err, "Could not send the pod log",
			"namespace", pod.Namespace,
			"pod", pod.Name,
		)
		return
	}
	metav1.SetMetaDataAnnotation(&pod.ObjectMeta, podLogTimestampKey+channel, newTS)
	_, err = k8sClient.CoreV1().Pods(pod.Namespace).Update(&pod)
	if err != nil {
		log.Error(
			err, "Cloud not update Pod",
			"namespace", pod.Namespace,
			"pod", pod.Name,
			"ts", newTS,
		)
		return
	}
}
