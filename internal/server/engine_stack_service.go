package server

import "github.com/battlewithbytes/pve-appstore/internal/engine"

// EngineStackService isolates stack operations from HTTP handlers.
type EngineStackService interface {
	StartStack(req engine.StackCreateRequest) (*engine.Job, error)
	ListStacksEnriched() ([]*engine.StackListItem, error)
	GetStackDetail(id string) (*engine.StackDetail, error)
	StartStackContainer(id string) error
	StopStackContainer(id string) error
	RestartStackContainer(id string) error
	UninstallStack(id string) (*engine.Job, error)
	EditStack(id string, req engine.EditRequest) (*engine.Job, error)
	ValidateStack(req engine.StackCreateRequest) map[string]interface{}
	GetStack(id string) (*engine.Stack, error)
}

func (s *defaultEngineService) StartStack(req engine.StackCreateRequest) (*engine.Job, error) {
	return s.eng.StartStack(req)
}
func (s *defaultEngineService) ListStacksEnriched() ([]*engine.StackListItem, error) {
	return s.eng.ListStacksEnriched()
}
func (s *defaultEngineService) GetStackDetail(id string) (*engine.StackDetail, error) {
	return s.eng.GetStackDetail(id)
}
func (s *defaultEngineService) StartStackContainer(id string) error {
	return s.eng.StartStackContainer(id)
}
func (s *defaultEngineService) StopStackContainer(id string) error {
	return s.eng.StopStackContainer(id)
}
func (s *defaultEngineService) RestartStackContainer(id string) error {
	return s.eng.RestartStackContainer(id)
}
func (s *defaultEngineService) UninstallStack(id string) (*engine.Job, error) {
	return s.eng.UninstallStack(id)
}
func (s *defaultEngineService) EditStack(id string, req engine.EditRequest) (*engine.Job, error) {
	return s.eng.EditStack(id, req)
}
func (s *defaultEngineService) ValidateStack(req engine.StackCreateRequest) map[string]interface{} {
	return s.eng.ValidateStack(req)
}
func (s *defaultEngineService) GetStack(id string) (*engine.Stack, error) {
	return s.eng.GetStack(id)
}
