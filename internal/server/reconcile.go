package server

import "log"

// ReconcileDevApps checks all published dev apps for merged PRs and
// auto-undeploys them when their PR has been merged into the catalog.
// Returns the list of app/stack IDs that were reconciled.
func (s *Server) ReconcileDevApps() []string {
	if s.devSvc == nil || s.catalogSvc == nil {
		return nil
	}

	var merged []string

	// Check dev apps
	apps, err := s.devSvc.List()
	if err != nil {
		log.Printf("[reconcile] failed to list dev apps: %v", err)
	} else {
		for _, app := range apps {
			// Clean up legacy "merged" status entries from before delete-on-merge
			if app.Status == "merged" {
				log.Printf("[reconcile] dev app %s: cleaning up stale merged entry", app.ID)
				s.catalogSvc.RemoveDevApp(app.ID)
				if err := s.devSvc.Delete(app.ID); err != nil {
					log.Printf("[reconcile] failed to delete dev app %s: %v", app.ID, err)
				}
				merged = append(merged, app.ID)
				continue
			}
			if (app.Status != "published" && app.Status != "deployed") || app.GitHubPRURL == "" {
				continue
			}
			state := s.getPRState(app.GitHubPRURL)
			if state == "pr_merged" {
				log.Printf("[reconcile] dev app %s: PR merged, removing dev app", app.ID)
				s.catalogSvc.RemoveDevApp(app.ID)
				if err := s.devSvc.Delete(app.ID); err != nil {
					log.Printf("[reconcile] failed to delete dev app %s: %v", app.ID, err)
				}
				merged = append(merged, app.ID)
			}
		}
	}

	// Check dev stacks
	stacks, err := s.devSvc.ListStacks()
	if err != nil {
		log.Printf("[reconcile] failed to list dev stacks: %v", err)
	} else {
		for _, stack := range stacks {
			if stack.Status == "merged" {
				log.Printf("[reconcile] dev stack %s: cleaning up stale merged entry", stack.ID)
				s.catalogSvc.RemoveDevStack(stack.ID)
				if err := s.devSvc.DeleteStack(stack.ID); err != nil {
					log.Printf("[reconcile] failed to delete dev stack %s: %v", stack.ID, err)
				}
				merged = append(merged, stack.ID)
				continue
			}
			if (stack.Status != "published" && stack.Status != "deployed") || stack.GitHubPRURL == "" {
				continue
			}
			state := s.getPRState(stack.GitHubPRURL)
			if state == "pr_merged" {
				log.Printf("[reconcile] dev stack %s: PR merged, removing dev stack", stack.ID)
				s.catalogSvc.RemoveDevStack(stack.ID)
				if err := s.devSvc.DeleteStack(stack.ID); err != nil {
					log.Printf("[reconcile] failed to delete dev stack %s: %v", stack.ID, err)
				}
				merged = append(merged, stack.ID)
			}
		}
	}

	if len(merged) > 0 {
		log.Printf("[reconcile] reconciled %d dev items: %v", len(merged), merged)
	}

	return merged
}

// tryRefreshInBackground triggers a catalog refresh + reconciliation in a
// background goroutine. Uses TryLock to prevent duplicate goroutines from
// rapid frontend polling — if a refresh is already running, the new trigger
// is silently dropped.
func (s *Server) tryRefreshInBackground(reason string) {
	go func() {
		if !s.refreshing.TryLock() {
			return // already refreshing
		}
		defer s.refreshing.Unlock()
		log.Printf("[catalog] background refresh: %s", reason)
		if err := s.RefreshAndReconcile(); err != nil {
			log.Printf("[catalog] background refresh failed: %v", err)
		}
	}()
}

// RefreshAndReconcile refreshes the catalog and then reconciles dev apps.
// Used by the auto-refresh goroutine.
func (s *Server) RefreshAndReconcile() error {
	if s.catalogSvc == nil {
		return nil
	}
	if err := s.catalogSvc.Refresh(); err != nil {
		return err
	}
	s.ReconcileDevApps()
	return nil
}
