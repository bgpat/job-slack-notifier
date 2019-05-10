package notifier

import (
	"fmt"
	"strings"
	"sync"

	"github.com/nlopes/slack"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

var (
	client *slack.Client

	cache   = map[types.UID]map[string]string{}
	cacheMu = sync.Map{}
)

// SetToken set the slack API token.
func SetToken(token string) {
	client = slack.New(token)
}

// Post sends a message to the specified channel.
func Post(channel, ts string, event watch.EventType, job *batchv1.Job) (newTS string, unlock func(), err error) {
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
		deleted = ":boom: *deleted*"
	}
	options := []slack.MsgOption{
		slack.MsgOptionUsername(fmt.Sprintf("%s/%s", job.Namespace, job.Name)),
		slack.MsgOptionText(strings.Join([]string{
			deleted,
			fmt.Sprintf(
				":arrow_right: *active* %d\t\t:heavy_check_mark: *succeeded* %d\t\t:heavy_multiplication_x: *failed* %d",
				job.Status.Active,
				job.Status.Succeeded,
				job.Status.Failed,
			),
			startTime,
			completionTime,
		}, "\n"), true),
		slack.MsgOptionAttachments(slack.Attachment{}),
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
