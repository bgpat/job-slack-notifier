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
)

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
	options := []slack.MsgOption{
		slack.MsgOptionUsername(n.username),
		slack.MsgOptionText(strings.Join(body, "\n"), false),
		//slack.MsgOptionAttachments(attachments...),
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
