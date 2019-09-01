package notifier

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/nlopes/slack"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var podStatuses = map[corev1.PodPhase]struct {
	color string
	icon  string
}{
	corev1.PodPending:   {color: "#FEE233", icon: "hourglass_flowing_sand"},
	corev1.PodRunning:   {color: "#66C0EA", icon: "arrow_right"},
	corev1.PodSucceeded: {color: "#84B74C", icon: "white_check_mark"},
	corev1.PodFailed:    {color: "#F76934", icon: "x"},
}

// Notification represents a slack message
type Notification struct {
	*Notifier

	channel string
	ts      string

	job     batchv1.Job
	cronJob batchv1beta1.CronJob
	pods    []corev1.Pod

	deleted bool

	init     sync.Once
	waitInit chan struct{}
}

type notificationKey struct {
	channel   string
	namespace string
	name      string
}

func (n *Notification) getMessageTimestamp() {
	ts, ok := n.job.GetAnnotations()[fmt.Sprintf(messageTimestampAttributeKey, n.channel)]
	if ok {
		n.ts = ts
	}
}

func (n *Notification) setMessageTimestamp() {
	ctx := context.Background()
	metav1.SetMetaDataAnnotation(
		&n.job.ObjectMeta,
		fmt.Sprintf(messageTimestampAttributeKey, n.channel),
		n.ts,
	)
	if err := n.jobClient.Update(ctx, &n.job); err != nil {
		logger.Error(
			err, "failed to update job annotation",
			"job", n.job.Namespace+"/"+n.job.Name,
			"channel", n.channel,
			"ts", n.ts,
		)
	}
}

func (n *Notification) updateMessage() error {
	logger.Info(
		"updateMessage",
		"status", n.job.Status,
	)
	ctx := context.Background()
	if err := n.jobClient.Get(ctx, client.ObjectKey{Namespace: n.job.Namespace, Name: n.job.Name}, &n.job); err != nil {
		logger.Error(err, "failed to get job")
		return err
	}
	statuses := []string{
		":arrow_right: *active* %d/%d",
		":white_check_mark: *succeeded* %d/%d",
		":x: *failed* %d/%d",
	}
	if n.deleted {
		statuses = append(statuses, ":boom: *deleted*")
	}
	body := []string{
		fmt.Sprintf("*%s/%s*", n.job.Namespace, n.job.Name),
		fmt.Sprintf(
			strings.Join(statuses, "\t\t"),
			n.job.Status.Active,
			*n.job.Spec.Parallelism,
			n.job.Status.Succeeded,
			*n.job.Spec.Completions,
			n.job.Status.Failed,
			*n.job.Spec.BackoffLimit+1,
		),
	}
	if n.job.Status.StartTime != nil {
		body = append(body, ":clock9: *start time*\t\t\t\t"+n.job.Status.StartTime.String())
	}
	if n.job.Status.CompletionTime != nil {
		body = append(body, ":clock5: *completion time*\t"+n.job.Status.CompletionTime.String())
	}
	if n.cronJob.Name != "" {
		body = append(body, fmt.Sprintf(":calendar: *schedule* (%s/%s)\t`%s`", n.cronJob.Namespace, n.cronJob.Name, n.cronJob.Spec.Schedule))
	}
	pods := make([]slack.Attachment, 0, len(n.pods))
	for _, p := range n.pods {
		text := []string{fmt.Sprintf(":%s: %s", podStatuses[p.Status.Phase].icon, string(p.Status.Phase))}
		if p.Status.Message != "" {
			text = append(text, fmt.Sprintf("*message*\t\t%q", p.Status.Message))
		}
		if p.Status.Reason != "" {
			text = append(text, fmt.Sprintf("*reason*\t\t%q", p.Status.Reason))
		}
		fields := []slack.AttachmentField{}
		containers := make([]corev1.ContainerStatus, 0, len(p.Status.InitContainerStatuses)+len(p.Status.ContainerStatuses))
		containers = append(containers, p.Status.InitContainerStatuses...)
		containers = append(containers, p.Status.ContainerStatuses...)
		for _, ct := range containers {
			var statuses []string
			switch {
			case ct.State.Waiting != nil:
				if ct.State.Waiting.Message != "" {
					statuses = append(statuses, "*message*\t\t"+ct.State.Waiting.Message)
				}
				if ct.State.Waiting.Reason != "" {
					statuses = append(statuses, "*reason*\t\t"+ct.State.Waiting.Reason)
				}
			case ct.State.Running != nil:
				statuses = append(statuses, fmt.Sprintf(
					":clock9: *started at*\t\t%v",
					ct.State.Running.StartedAt,
				))
			case ct.State.Terminated != nil:
				statuses = []string{
					fmt.Sprintf(":clock9: *started at*\t\t  %v", ct.State.Terminated.StartedAt),
					fmt.Sprintf(":clock5: *finished at*\t\t%v", ct.State.Terminated.FinishedAt),
					fmt.Sprintf(
						strings.Join([]string{
							":door: *exit code* %d",
							":traffic_light: *signal* %d",
						}, "\t\t"),
						ct.State.Terminated.ExitCode,
						ct.State.Terminated.Signal,
					),
				}
				if ct.State.Terminated.Reason != "" {
					statuses = append(statuses, ":bulb: *reason*\t"+ct.State.Terminated.Reason)
				}
				if ct.State.Terminated.Message != "" {
					statuses = append(statuses, ":memo: *message*\t"+ct.State.Terminated.Message)
				}
			}
			fields = append(fields, slack.AttachmentField{
				Title: ct.Name,
				Value: strings.Join(statuses, "\n"),
			})
		}
		a := slack.Attachment{
			Color:  podStatuses[p.Status.Phase].color,
			Text:   strings.Join(text, "\n"),
			Title:  p.Name,
			Fields: fields,
			Footer: fmt.Sprintf("created at %v", p.CreationTimestamp),
		}
		pods = append(pods, a)
	}
	options := []slack.MsgOption{
		slack.MsgOptionUsername(n.username),
		slack.MsgOptionText(strings.Join(body, "\n"), false),
		slack.MsgOptionAttachments(pods...),
	}
	if n.ts != "" {
		options = append(options, slack.MsgOptionUpdate(n.ts))
	}
	_, newTS, _, err := n.client.SendMessage(n.channel, options...)
	if err != nil {
		return err
	}
	if n.ts != newTS {
		n.ts = newTS
		if err := n.setNotification(n); err != nil {
			logger.Error(
				err, "failed to set message timestamp annotation",
				"key", fmt.Sprintf(messageTimestampAttributeKey, n.channel),
				"timestamp", n.ts,
			)
			return err
		}
	}
	return nil
}
