package notifier

import (
	"context"

	jsnv1beta1 "github.com/bgpat/job-slack-notifier/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	messageTimestampAttributeKey = jsnv1beta1.GroupVersion.Group + "/%s.message"
)

// NotifyJob updates the notification of the job.
func NotifyJob(req client.ObjectKey) {
	DefaultNotifier.notifyJob(req)
}

func (n *Notifier) notifyJob(req client.ObjectKey) {
	logger := logger.WithValues("job", req)

	job, err := n.getJob(req)
	if errors.IsNotFound(err) {
		logger.Info("deleted")
		for _, n := range n.getAllNotifications(req) {
			n := n
			go func() {
				n.deleted = true
				if err := n.updateMessage(); err != nil {
					logger.Error(err, "failed to update a message")
				}
			}()
		}
		return
	} else if err != nil {
		logger.Error(err, "failed to get job")
		return
	}

	cj, err := n.ownerCronJob(job)
	if err != nil {
		logger.Info("failed to get owner cronJob", "error", err)
	}
	logger.Info("found owner cronJob", "cronJob", cj.Name)

	pods, err := n.childrenPods(job)
	if err != nil {
		logger.Info("failed to get children pods", "error", err)
	}
	podNames := make([]string, 0, len(pods))
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	logger.Info("found children pods", "pods", podNames)

	jns, err := n.searchNotifiers(job)
	if err != nil {
		logger.Error(err, "failed to search the matched notifier")
		return
	}
	for _, jn := range jns {
		logger := logger.WithValues("jobNotifier", client.ObjectKey{
			Namespace: jn.Namespace,
			Name:      jn.Name,
		})
		for _, name := range jn.Spec.Channels {
			ch, err := n.channelID(name)
			if err != nil {
				logger.Error(err, "failed to get chennel info", "channelName", name)
				continue
			}
			logger := logger.WithValues("channel", ch)
			logger.Info("found channel")
			notification := n.getNotification(ch, job)
			if cj.Name != "" {
				notification.cronJob = cj
			}
			if pods != nil {
				notification.pods = pods
			}
			go func() {
				err = notification.updateMessage()
				if err != nil {
					logger.Error(err, "failed to send a message")
					return
				}
			}()
		}
	}
}

func (n *Notifier) getJob(req client.ObjectKey) (job batchv1.Job, err error) {
	ctx := context.Background()
	err = n.jobClient.Get(ctx, req, &job)
	return
}

func (n *Notifier) ownerCronJob(job batchv1.Job) (cj batchv1beta1.CronJob, err error) {
	ctx := context.Background()
	owner := metav1.GetControllerOf(&job)
	if owner != nil &&
		owner.APIVersion == batchv1beta1.SchemeGroupVersion.String() &&
		owner.Kind == "CronJob" {
		err = client.IgnoreNotFound(n.cronJobClient.Get(ctx, client.ObjectKey{
			Namespace: job.Namespace,
			Name:      owner.Name,
		}, &cj))
	}
	return
}

func (n *Notifier) childrenPods(job batchv1.Job) ([]corev1.Pod, error) {
	ctx := context.Background()
	var list corev1.PodList
	pods := make([]corev1.Pod, 0)
	err := n.podClient.List(ctx, &list, client.InNamespace(job.Namespace))
	if err != nil {
		return nil, err
	}
	for _, pod := range list.Items {
		owner := metav1.GetControllerOf(&pod)
		if owner == nil {
			continue
		}
		if owner.APIVersion != corev1.SchemeGroupVersion.String() || owner.Kind != "Job" {
			continue
		}
		if owner.Name == job.Name {
			pods = append(pods, pod)
		}
	}
	return pods, nil
}
