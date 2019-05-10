package notifier

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nlopes/slack"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

var (
	client *slack.Client

	cache   = map[types.UID]map[string]string{}
	cacheMu = sync.Map{}

	podStatuses = map[corev1.PodPhase]struct {
		color string
		icon  string
	}{
		corev1.PodPending:   {color: "#FEE233", icon: "hourglass_flowing_sand"},
		corev1.PodRunning:   {color: "#66C0EA", icon: "arrow_right"},
		corev1.PodSucceeded: {color: "#84B74C", icon: "heavy_check_mark"},
		corev1.PodFailed:    {color: "#F76934", icon: "heavy_multiplication_x"},
	}
)

// SetToken set the slack API token.
func SetToken(token string) {
	client = slack.New(token)
}

// Post sends a message to the specified channel.
func Post(channel, ts string, event watch.EventType, job *batchv1.Job, pods []corev1.Pod) (newTS string, unlock func(), err error) {
	if ts == "" {
		ts = getTS(channel, job.UID)
	}
	if ts == "" {
		unlock = func() {
			setTS(channel, job.UID)(newTS)
		}
	}

	var startTime, completionTime, deleted string
	if job.Status.StartTime != nil {
		startTime = ":clock9: *start time*\t\t\t\t" + job.Status.StartTime.String()
	}
	if job.Status.CompletionTime != nil {
		completionTime = ":clock5: *completion time*\t" + job.Status.CompletionTime.String()
	}
	if event == watch.Deleted {
		deleted = "\t\t:boom: *deleted*"
	}
	attachments := make([]slack.Attachment, 0, len(pods))
	for _, pod := range pods {
		text := []string{fmt.Sprintf(":%s: %s", podStatuses[pod.Status.Phase].icon, string(pod.Status.Phase))}
		if pod.Status.Message != "" {
			text = append(text, fmt.Sprintf("*message*\t\t%q", pod.Status.Message))
		}
		if pod.Status.Reason != "" {
			text = append(text, fmt.Sprintf("*reason*\t\t%q", pod.Status.Reason))
		}
		fields := []slack.AttachmentField{}
		containers := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
		containers = append(containers, pod.Status.InitContainerStatuses...)
		containers = append(containers, pod.Status.ContainerStatuses...)
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
					fmt.Sprintf(":clock9: *started at*\t\t%v", ct.State.Terminated.StartedAt),
					fmt.Sprintf(":clock5: *finished at*\t\t%v", ct.State.Terminated.FinishedAt),
					fmt.Sprintf(
						"*exit code* %d\t\t*signal* %d",
						ct.State.Terminated.ExitCode,
						ct.State.Terminated.Signal,
					),
				}
				if ct.State.Terminated.Message != "" {
					statuses = append(statuses, "*message*\t\t"+ct.State.Terminated.Message)
				}
				if ct.State.Terminated.Reason != "" {
					statuses = append(statuses, "*reason*\t\t"+ct.State.Terminated.Reason)
				}
			}
			fields = append(fields, slack.AttachmentField{
				Title: ct.Name,
				Value: strings.Join(statuses, "\n"),
			})
		}
		attachments = append(attachments, slack.Attachment{
			Color:  podStatuses[pod.Status.Phase].color,
			Title:  pod.Name,
			Text:   strings.Join(text, "\n"),
			Fields: fields,
			Footer: fmt.Sprintf("%v", pod.Status.StartTime),
		})
	}
	options := []slack.MsgOption{
		slack.MsgOptionUsername(fmt.Sprintf("%s/%s", job.Namespace, job.Name)),
		slack.MsgOptionText(strings.Join([]string{
			fmt.Sprintf(
				":arrow_right: *active* %d\t\t:heavy_check_mark: *succeeded* %d\t\t:heavy_multiplication_x: *failed* %d%s",
				job.Status.Active,
				job.Status.Succeeded,
				job.Status.Failed,
				deleted,
			),
			startTime,
			completionTime,
		}, "\n"), true),
		slack.MsgOptionAttachments(attachments...),
	}
	if ts != "" {
		options = append(options, slack.MsgOptionUpdate(ts))
	}
	_, newTS, _, err = client.SendMessage(channel, options...)
	return
}

func getTS(channel string, uid types.UID) string {
	v, _ := cacheMu.LoadOrStore(uid, &sync.RWMutex{})
	mu := v.(*sync.RWMutex)
	mu.RLock()
	defer mu.RUnlock()
	jc, ok := cache[uid]
	if !ok {
		mu.RUnlock()
		mu.Lock()
		cache[uid] = map[string]string{}
		jc = cache[uid]
		mu.Unlock()
		mu.RLock()
	}
	return jc[channel]
}

func setTS(channel string, uid types.UID) func(string) {
	v, _ := cacheMu.Load(uid)
	mu := v.(*sync.RWMutex)
	mu.Lock()
	return func(ts string) {
		cache[uid][channel] = ts
		mu.Unlock()
	}
}
