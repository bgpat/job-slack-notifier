package notifier

import (
	"fmt"
	"time"

	"github.com/nlopes/slack"
)

func (n *Notifier) channelID(name string) (string, error) {
	now := time.Now()
	n.channelCacheMu.RLock()
	expired := now.Sub(n.channelCachedAt) > channelCacheTTL
	n.channelCacheMu.RUnlock()
	if expired {
		n.channelCacheMu.Lock()
		channels, err := n.client.GetChannels(true, slack.GetChannelsOptionExcludeMembers())
		if err != nil {
			return "", err
		}
		n.channelIDs = make(map[string]string)
		n.channelNames = make(map[string]string)
		for _, ch := range channels {
			n.channelIDs[ch.ID] = ch.ID
			n.channelNames[ch.Name] = ch.ID
		}
		n.channelCachedAt = now
		logger.Info("updated slack channels cache", "ids", n.channelIDs, "names", n.channelNames)
		n.channelCacheMu.Unlock()
	}

	if id, ok := n.channelIDs[name]; ok {
		return id, nil
	}
	if id, ok := n.channelNames[name]; ok {
		return id, nil
	}
	return "", fmt.Errorf("slack channel %q is not found", name)
}
