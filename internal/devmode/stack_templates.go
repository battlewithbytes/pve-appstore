package devmode

import "fmt"

// GenerateStackManifest returns the stack.yml content for a new dev stack.
func GenerateStackManifest(id, name, template string) string {
	switch template {
	case "web-database":
		return webDatabaseStackManifest(id, name)
	default:
		return blankStackManifest(id, name)
	}
}

func blankStackManifest(id, name string) string {
	return fmt.Sprintf(`id: %s
name: "%s"
description: "TODO: Describe your stack"
version: "0.1.0"
categories:
  - utilities
tags: []

apps:
  - app_id: my-app
    # inputs:
    #   key: value

lxc:
  ostemplate: debian-12
  defaults:
    cores: 2
    memory_mb: 1024
    disk_gb: 8
`, id, name)
}

func webDatabaseStackManifest(id, name string) string {
	return fmt.Sprintf(`id: %s
name: "%s"
description: "Web server with database backend"
version: "0.1.0"
categories:
  - web
  - database
tags:
  - stack
  - web-database

apps:
  - app_id: nginx
  - app_id: postgres
    inputs:
      db_name: webapp

lxc:
  ostemplate: debian-12
  defaults:
    cores: 2
    memory_mb: 1024
    disk_gb: 8
`, id, name)
}
