package server

import "github.com/battlewithbytes/pve-appstore/internal/engine"

// EngineConfigService isolates config import/export engine operations.
type EngineConfigService interface {
	StartInstall(req engine.InstallRequest) (*engine.Job, error)
	StartStack(req engine.StackCreateRequest) (*engine.Job, error)
	ListInstalls() ([]*engine.Install, error)
	ListStacks() ([]*engine.Stack, error)
}

// EngineService is the full default engine service surface.
type EngineService interface {
	EngineInstallService
	EngineStackService
	EngineConfigService
}

type defaultEngineService struct {
	eng *engine.Engine
}

func NewEngineService(eng *engine.Engine) EngineService {
	if eng == nil {
		return nil
	}
	return &defaultEngineService{eng: eng}
}

func (s *defaultEngineService) ListInstalls() ([]*engine.Install, error) {
	return s.eng.ListInstalls()
}
func (s *defaultEngineService) ListStacks() ([]*engine.Stack, error) {
	return s.eng.ListStacks()
}
