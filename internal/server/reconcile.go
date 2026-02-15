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
			if app.Status != "published" || app.GitHubPRURL == "" {
				continue
			}
			state := s.getPRState(app.GitHubPRURL)
			if state == "pr_merged" {
				log.Printf("[reconcile] dev app %s: PR merged, auto-undeploying", app.ID)
				s.catalogSvc.RemoveDevApp(app.ID)
				s.devSvc.SetGitHubMeta(app.ID, map[string]string{"status": "merged"})
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
			if stack.Status != "published" || stack.GitHubPRURL == "" {
				continue
			}
			state := s.getPRState(stack.GitHubPRURL)
			if state == "pr_merged" {
				log.Printf("[reconcile] dev stack %s: PR merged, auto-undeploying", stack.ID)
				s.catalogSvc.RemoveDevStack(stack.ID)
				s.devSvc.SetGitHubMeta(stack.ID, map[string]string{"status": "merged"})
				merged = append(merged, stack.ID)
			}
		}
	}

	if len(merged) > 0 {
		log.Printf("[reconcile] reconciled %d dev items: %v", len(merged), merged)
	}

	return merged
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
