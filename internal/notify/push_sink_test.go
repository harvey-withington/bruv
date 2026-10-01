package notify

import (
	"testing"
	"time"

	"bruv/internal/config"
)

// Every in-app notification is mirrored to the push sink a BRUV Server
// installs, so phones that opted into Web Push hear about agent results,
// due dates and alarms. Channels other than in-app don't reach it.
func TestInAppNotificationsReachPushSink(t *testing.T) {
	got := make(chan config.Notification, 4)
	SetPushSink(func(n config.Notification) { got <- n })
	t.Cleanup(func() { SetPushSink(nil) })

	d := NewDispatcher(config.NotifyConfig{}, nil)
	d.Send(Request{Title: "Agent finished", Body: "3 findings", CardID: "card-1", Channels: []Channel{ChannelInApp}})

	select {
	case n := <-got:
		if n.Title != "Agent finished" || n.CardID != "card-1" {
			t.Errorf("sink got %+v", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-app notification never reached the push sink")
	}

	// A system-only notification isn't an in-app one, so it isn't pushed.
	d.Send(Request{Title: "system only", Channels: []Channel{ChannelSystem}})
	select {
	case n := <-got:
		t.Errorf("non-in-app notification was pushed: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
}

// Without a sink (the desktop), in-app notifications still work.
func TestInAppWithoutPushSink(t *testing.T) {
	SetPushSink(nil)
	d := NewDispatcher(config.NotifyConfig{}, nil)
	d.Send(Request{Title: "no sink", Channels: []Channel{ChannelInApp}})
}
