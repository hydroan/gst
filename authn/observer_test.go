package authn_test

import (
	"testing"
	"time"

	"github.com/hydroan/gst/authn"
	"github.com/hydroan/gst/types"
	"github.com/stretchr/testify/require"
)

func sampleLoginEvent() authn.LoginEvent {
	return authn.LoginEvent{
		Kind:           authn.LoginEventSucceeded,
		UserID:         "user-1",
		Username:       "sample",
		TenantID:       "tenant-1",
		ClientIP:       "192.0.2.10",
		UserAgent:      "sample-agent/1.0",
		OS:             "Sample OS",
		Platform:       "Sample Platform",
		EngineName:     "SampleEngine",
		EngineVersion:  "1.0",
		BrowserName:    "SampleBrowser",
		BrowserVersion: "2.0",
		At:             time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func TestAddLoginObserverMulticasts(t *testing.T) {
	ctx := &types.ServiceContext{}
	event := sampleLoginEvent()

	var first []authn.LoginEvent
	removeFirst := authn.AddLoginObserver(func(gotCtx *types.ServiceContext, got authn.LoginEvent) {
		require.Same(t, ctx, gotCtx)
		first = append(first, got)
	})
	t.Cleanup(removeFirst)

	var second []authn.LoginEvent
	removeSecond := authn.AddLoginObserver(func(_ *types.ServiceContext, got authn.LoginEvent) {
		second = append(second, got)
	})
	t.Cleanup(removeSecond)

	authn.NotifyLogin(ctx, event)

	require.Equal(t, []authn.LoginEvent{event}, first)
	require.Equal(t, []authn.LoginEvent{event}, second)
}

func TestAddLoginObserverRemoveStopsDelivery(t *testing.T) {
	var events []authn.LoginEvent
	remove := authn.AddLoginObserver(func(_ *types.ServiceContext, got authn.LoginEvent) {
		events = append(events, got)
	})

	authn.NotifyLogin(&types.ServiceContext{}, sampleLoginEvent())
	require.Len(t, events, 1)

	remove()
	authn.NotifyLogin(&types.ServiceContext{}, sampleLoginEvent())
	require.Len(t, events, 1)

	// Removal is idempotent.
	remove()
	authn.NotifyLogin(&types.ServiceContext{}, sampleLoginEvent())
	require.Len(t, events, 1)
}

func TestNotifyLoginRecoversPanickingObserver(t *testing.T) {
	removePanicking := authn.AddLoginObserver(func(*types.ServiceContext, authn.LoginEvent) {
		panic("sample observer failure")
	})
	t.Cleanup(removePanicking)

	var events []authn.LoginEvent
	removeHealthy := authn.AddLoginObserver(func(_ *types.ServiceContext, got authn.LoginEvent) {
		events = append(events, got)
	})
	t.Cleanup(removeHealthy)

	require.NotPanics(t, func() {
		authn.NotifyLogin(&types.ServiceContext{}, sampleLoginEvent())
	})
	require.Len(t, events, 1)
}

func TestAddLoginObserverNilIsNoop(t *testing.T) {
	remove := authn.AddLoginObserver(nil)
	require.NotPanics(t, func() {
		remove()
		authn.NotifyLogin(&types.ServiceContext{}, sampleLoginEvent())
	})
}
