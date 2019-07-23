package notifier

import (
	"context"
	"os"
	"sync"
	"time"

	jsnv1beta1 "github.com/bgpat/job-slack-notifier/api/v1beta1"
	"github.com/nlopes/slack"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	slackTokenEnvKey = "SLACK_TOKEN"
	channelCacheTTL  = time.Hour
)

// DefaultNotifier is the default Notifier.
var DefaultNotifier *Notifier

var logger = ctrl.Log.WithName("notifier")

// Notifier is slack notifier.
type Notifier struct {
	jobNotifierClient client.Client
	podClient         client.Client
	jobClient         client.Client
	cronJobClient     client.Client

	client   *slack.Client
	username string

	channelCachedAt time.Time
	channelIDs      map[string]string
	channelNames    map[string]string
	channelCacheMu  sync.RWMutex

	notifications sync.Map
}

// NewNotifier returns the slack notifier.
func NewNotifier(jobNotifierClient, podClient, jobClient, cronJobClient client.Client, token, username string) *Notifier {
	if token == "" {
		token = os.Getenv(slackTokenEnvKey)
	}
	return &Notifier{
		jobNotifierClient: jobNotifierClient,
		podClient:         podClient,
		jobClient:         jobClient,
		cronJobClient:     cronJobClient,
		client:            slack.New(token),
		username:          username,
	}
}

func (n *Notifier) searchNotifiers(job batchv1.Job) (jns []jsnv1beta1.JobNotifier, err error) {
	ctx := context.Background()
	var l jsnv1beta1.JobNotifierList
	err = DefaultNotifier.jobNotifierClient.List(ctx, &l, client.InNamespace(job.Namespace))
	if err != nil {
		return
	}
	for _, jn := range l.Items {
		selector, err := metav1.LabelSelectorAsSelector(jn.Spec.Selector)
		if err != nil {
			return nil, err
		}
		if selector.Matches(labels.Set(job.Labels)) {
			jns = append(jns, jn)
		}
	}
	return
}

func (n *Notifier) getNotification(ch string, job batchv1.Job) (notification *Notification) {
	key := notificationKey{
		channel:   ch,
		namespace: job.Namespace,
		name:      job.Name,
	}
	v, ok := n.notifications.LoadOrStore(key, &Notification{
		Notifier: n,
		channel:  ch,
		job:      job,
		waitInit: make(chan struct{}, 0),
	})
	notification = v.(*Notification)
	if ok {
		<-notification.waitInit
		v, _ := n.notifications.Load(key)
		notification = v.(*Notification)
	} else {
		notification.getMessageTimestamp()
	}
	return notification
}

func (n *Notifier) setNotification(notification *Notification) (err error) {
	notification.init.Do(func() {
		key := notificationKey{
			channel:   notification.channel,
			namespace: notification.job.Namespace,
			name:      notification.job.Name,
		}
		notification.setMessageTimestamp()
		n.notifications.Store(key, notification)
		close(notification.waitInit)
	})
	return
}

func (n *Notifier) getAllNotifications(job client.ObjectKey) []*Notification {
	notifications := make([]*Notification, 0)
	n.notifications.Range(func(k, v interface{}) bool {
		key := k.(notificationKey)
		if key.namespace == job.Namespace && key.name == job.Name {
			notifications = append(notifications, v.(*Notification))
		}
		return true
	})
	return notifications
}
