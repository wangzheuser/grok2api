package app

import (
	"context"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/model"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

type readinessRoutes struct {
	repository.ModelRepository
}

// ListConfiguredEnabled 模拟共享额度池的多个模型路由。
func (readinessRoutes) ListConfiguredEnabled(context.Context) ([]model.Route, error) {
	var routes []model.Route
	for i := uint64(1); i <= 5; i++ {
		routes = append(routes, model.Route{ID: i, Provider: account.ProviderConsole, SupportedAccounts: 1})
	}
	return routes, nil
}

type readinessPool struct {
	repository.AccountRepository
	repository.RoutingLayerRepository
	baseCalls, candidateCalls int
	remaining                 int
}

// ListRoutingAccountBases 只返回一个额度池快照。
func (p *readinessPool) ListRoutingAccountBases(context.Context, account.Provider, string) ([]account.RoutingAccountBase, error) {
	p.baseCalls++
	return []account.RoutingAccountBase{{
		Credential:  account.Credential{ID: 1, Provider: account.ProviderConsole, AuthType: account.AuthTypeSSO, AuthStatus: account.AuthStatusActive},
		QuotaWindow: &account.QuotaWindow{Remaining: p.remaining},
	}}, nil
}

// ListRoutingCandidates 保留模型级能力判断，基础额度可用不等于模型可用。
func (p *readinessPool) ListRoutingCandidates(context.Context, account.Provider, uint64, string, string) ([]account.RoutingCandidate, error) {
	p.candidateCalls++
	return nil, nil
}

// TestReadinessSkipsModelScansForExhaustedPool 防止逐路由反复扫描数千个额度耗尽账号。
func TestReadinessSkipsModelScansForExhaustedPool(t *testing.T) {
	pool := &readinessPool{}
	state := newStartupState(0)
	state.setPhase("running")
	got := readinessSnapshot(context.Background(), state, func(context.Context) error { return nil }, readinessRoutes{}, pool, provider.NewRegistry(), nil)
	if got.Ready || got.Components["grok_console"].State != "unavailable" {
		t.Fatalf("snapshot=%+v", got)
	}
	if pool.baseCalls != 1 || pool.candidateCalls != 0 {
		t.Fatalf("base scans=%d model scans=%d, want 1/0", pool.baseCalls, pool.candidateCalls)
	}
	// 每次探测重新读取状态，不缓存上一轮的额度耗尽结论。
	pool.remaining = 1
	_ = readinessSnapshot(context.Background(), state, func(context.Context) error { return nil }, readinessRoutes{}, pool, provider.NewRegistry(), nil)
	if pool.baseCalls != 2 || pool.candidateCalls != 5 {
		t.Fatalf("base scans=%d model scans=%d, want 2/5 after recovery", pool.baseCalls, pool.candidateCalls)
	}
}
