package agent

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	pb "github.com/nginx/agent/v3/api/grpc/mpi/v1"
	"github.com/nginx/nginx-gateway-fabric/v2/internal/controller/nginx/agent/broadcast/broadcastfakes"
	"github.com/nginx/nginx-gateway-fabric/v2/internal/controller/status"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Adapted from upstream PR 6039 to the v2.6.7 generated fake API.
func TestUpdateConfig_RetriesAfterFailedApply(t *testing.T) {
	broadcaster := &broadcastfakes.FakeBroadcaster{}
	broadcaster.SendReturns(true)
	updater := NewNginxUpdater(logr.Discard(), fake.NewFakeClient(), &status.Queue{}, nil, false)
	deployment := &Deployment{broadcaster: broadcaster, podStatuses: make(map[string]error)}
	files := []File{{Meta: &pb.FileMeta{Name: "test.conf", Hash: "12345"}, Contents: []byte("test content")}}
	applyErr := errors.New("transient NGINX validation failure")
	deployment.SetPodErrorStatus("pod1", applyErr)
	updater.UpdateConfig(deployment, files, nil)
	if broadcaster.SendCallCount() != 1 {
		t.Fatalf("initial sends = %d, want 1", broadcaster.SendCallCount())
	}
	updater.UpdateConfig(deployment, files, nil)
	if broadcaster.SendCallCount() != 2 {
		t.Fatalf("unchanged failed config sends = %d, want 2", broadcaster.SendCallCount())
	}
	deployment.SetPodErrorStatus("pod1", nil)
	updater.UpdateConfig(deployment, files, nil)
	if broadcaster.SendCallCount() != 3 {
		t.Fatalf("recovery sends = %d, want 3", broadcaster.SendCallCount())
	}
	if err := deployment.GetLatestConfigError(); err != nil {
		t.Fatalf("stale error after recovery: %v", err)
	}
	updater.UpdateConfig(deployment, files, nil)
	if broadcaster.SendCallCount() != 3 {
		t.Fatalf("successful unchanged config resent: %d", broadcaster.SendCallCount())
	}
}

func TestUpdateConfig_RetriesSameVersionAfterNewPodInitialApplyFailure(t *testing.T) {
	broadcaster := &broadcastfakes.FakeBroadcaster{}
	broadcaster.SendReturns(true)
	updater := NewNginxUpdater(logr.Discard(), fake.NewFakeClient(), &status.Queue{}, nil, false)
	deployment := &Deployment{broadcaster: broadcaster, podStatuses: make(map[string]error)}
	files := []File{{Meta: &pb.FileMeta{Name: "test.conf", Hash: "12345"}, Contents: []byte("test content")}}
	updater.UpdateConfig(deployment, files, nil)
	deployment.SetPodErrorStatus("new-pod", errors.New("initial subscription apply failed"))
	updater.UpdateConfig(deployment, files, nil)
	if broadcaster.SendCallCount() != 2 {
		t.Fatalf("previously successful config sends after new pod failure = %d, want 2", broadcaster.SendCallCount())
	}
	deployment.SetPodErrorStatus("new-pod", nil)
	updater.UpdateConfig(deployment, files, nil)
	if err := deployment.GetLatestConfigError(); err != nil {
		t.Fatalf("stale error after recovery: %v", err)
	}
}
