package server

import "github.com/battlewithbytes/pve-appstore/internal/engine"

// EngineInstallService isolates install/job/runtime operations from HTTP handlers.
type EngineInstallService interface {
	StartInstall(req engine.InstallRequest) (*engine.Job, error)
	HasActiveInstallForApp(appID string) (*engine.Install, bool)
	HasActiveJobForApp(appID string) (*engine.Job, bool)
	CancelJob(id string) error
	ClearJobs() (int64, error)
	ListJobs() ([]*engine.Job, error)
	GetJob(id string) (*engine.Job, error)
	GetLogsSince(id string, afterID int) ([]*engine.LogEntry, int, error)
	ListInstallsEnriched() ([]*engine.InstallListItem, error)
	StartContainer(id string) error
	StopContainer(id string) error
	RestartContainer(id string) error
	GetInstallDetail(id string) (*engine.InstallDetail, error)
	GetInstall(id string) (*engine.Install, error)
	Uninstall(id string, keepVolumes bool) (*engine.Job, error)
	PurgeInstall(id string) error
	Reinstall(id string, req engine.ReinstallRequest) (*engine.Job, error)
	Update(id string, req engine.ReinstallRequest) (*engine.Job, error)
	EditInstall(id string, req engine.EditRequest) (*engine.Job, error)
	ReconfigureInstall(id string, req engine.ReconfigureRequest) (*engine.Install, error)
}

func (s *defaultEngineService) StartInstall(req engine.InstallRequest) (*engine.Job, error) {
	return s.eng.StartInstall(req)
}
func (s *defaultEngineService) HasActiveInstallForApp(appID string) (*engine.Install, bool) {
	return s.eng.HasActiveInstallForApp(appID)
}
func (s *defaultEngineService) HasActiveJobForApp(appID string) (*engine.Job, bool) {
	return s.eng.HasActiveJobForApp(appID)
}
func (s *defaultEngineService) CancelJob(id string) error { return s.eng.CancelJob(id) }
func (s *defaultEngineService) ClearJobs() (int64, error) { return s.eng.ClearJobs() }
func (s *defaultEngineService) ListJobs() ([]*engine.Job, error) {
	return s.eng.ListJobs()
}
func (s *defaultEngineService) GetJob(id string) (*engine.Job, error) { return s.eng.GetJob(id) }
func (s *defaultEngineService) GetLogsSince(id string, afterID int) ([]*engine.LogEntry, int, error) {
	return s.eng.GetLogsSince(id, afterID)
}
func (s *defaultEngineService) ListInstallsEnriched() ([]*engine.InstallListItem, error) {
	return s.eng.ListInstallsEnriched()
}
func (s *defaultEngineService) StartContainer(id string) error { return s.eng.StartContainer(id) }
func (s *defaultEngineService) StopContainer(id string) error  { return s.eng.StopContainer(id) }
func (s *defaultEngineService) RestartContainer(id string) error {
	return s.eng.RestartContainer(id)
}
func (s *defaultEngineService) GetInstallDetail(id string) (*engine.InstallDetail, error) {
	return s.eng.GetInstallDetail(id)
}
func (s *defaultEngineService) GetInstall(id string) (*engine.Install, error) {
	return s.eng.GetInstall(id)
}
func (s *defaultEngineService) Uninstall(id string, keepVolumes bool) (*engine.Job, error) {
	return s.eng.Uninstall(id, keepVolumes)
}
func (s *defaultEngineService) PurgeInstall(id string) error { return s.eng.PurgeInstall(id) }
func (s *defaultEngineService) Reinstall(id string, req engine.ReinstallRequest) (*engine.Job, error) {
	return s.eng.Reinstall(id, req)
}
func (s *defaultEngineService) Update(id string, req engine.ReinstallRequest) (*engine.Job, error) {
	return s.eng.Update(id, req)
}
func (s *defaultEngineService) EditInstall(id string, req engine.EditRequest) (*engine.Job, error) {
	return s.eng.EditInstall(id, req)
}
func (s *defaultEngineService) ReconfigureInstall(id string, req engine.ReconfigureRequest) (*engine.Install, error) {
	return s.eng.ReconfigureInstall(id, req)
}
