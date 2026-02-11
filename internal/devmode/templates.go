package devmode

import (
	"fmt"
	"strings"
)

// Template defines a starter app template.
type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// GenerateManifest returns the app.yml content for this template.
func (t *Template) GenerateManifest(appID string) string {
	name := titleFromID(appID)
	switch t.ID {
	case "web-server":
		return webServerManifest(appID, name)
	case "database":
		return databaseManifest(appID, name)
	case "python-app":
		return pythonAppManifest(appID, name)
	case "docker-import":
		return dockerImportManifest(appID, name)
	default:
		return blankManifest(appID, name)
	}
}

// GenerateScript returns the install.py content for this template.
func (t *Template) GenerateScript(appID string) string {
	className := toPascalCase(appID)
	switch t.ID {
	case "web-server":
		return webServerScript(className)
	case "database":
		return databaseScript(className)
	case "python-app":
		return pythonAppScript(className)
	case "docker-import":
		return dockerImportScript(className)
	default:
		return blankScript(className)
	}
}

var templates = []Template{
	{ID: "blank", Name: "Blank", Description: "Minimal valid manifest with an empty install method", Category: "General"},
	{ID: "web-server", Name: "Web Server", Description: "Nginx-based web server with config and service management", Category: "Web"},
	{ID: "database", Name: "Database", Description: "PostgreSQL-like database with user creation and config", Category: "Database"},
	{ID: "python-app", Name: "Python App", Description: "Python application with pip, venv, and systemd service", Category: "Development"},
}

// ListTemplates returns all available templates.
func ListTemplates() []Template {
	return templates
}

// GetTemplate returns a template by ID, or nil if not found.
func GetTemplate(id string) *Template {
	for i := range templates {
		if templates[i].ID == id {
			return &templates[i]
		}
	}
	return nil
}

func toPascalCase(id string) string {
	words := strings.Split(id, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, "")
}

func blankManifest(id, name string) string {
	return fmt.Sprintf(`id: %s
name: "%s"
description: "TODO: Describe your app"
version: "0.1.0"
categories:
  - utilities
tags: []
maintainers:
  - "Your Name"

lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    unprivileged: true
    cores: 1
    memory_mb: 512
    disk_gb: 4
    onboot: true

provisioning:
  script: provision/install.py
  timeout_sec: 300

outputs:
  - key: url
    label: "Access URL"
    value: "http://{{IP}}/"
`, id, name)
}

func blankScript(className string) string {
	return fmt.Sprintf(`#!/usr/bin/env python3
"""Provisioning script for %s."""
from appstore import BaseApp, run


class %s(BaseApp):
    def install(self):
        # TODO: Add your provisioning logic here
        self.log.info("Installation complete!")


run(%s)
`, className, className, className)
}

func webServerManifest(id, name string) string {
	return fmt.Sprintf(`id: %s
name: "%s"
description: "TODO: Describe your web server app"
version: "0.1.0"
categories:
  - web
tags:
  - nginx
  - web-server
maintainers:
  - "Your Name"

lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    unprivileged: true
    cores: 1
    memory_mb: 512
    disk_gb: 4
    onboot: true

inputs:
  - key: http_port
    label: "HTTP Port"
    type: number
    default: 80
    required: true
    validation:
      min: 1
      max: 65535
    help: "Port for the web server to listen on"

provisioning:
  script: provision/install.py
  timeout_sec: 300
  redact_keys: []

permissions:
  packages:
    - nginx
  services:
    - nginx
  paths:
    - /etc/nginx
    - /var/www/html

outputs:
  - key: url
    label: "Web URL"
    value: "http://{{IP}}:{{http_port}}"
`, id, name)
}

func webServerScript(className string) string {
	return fmt.Sprintf(`#!/usr/bin/env python3
"""Provisioning script for %s."""
from appstore import BaseApp, run


class %s(BaseApp):
    def install(self):
        http_port = self.inputs.integer("http_port", 80)

        # Install nginx
        self.apt_install(["nginx"])

        # Write a simple config
        config = f"""
server {{
    listen {http_port} default_server;
    root /var/www/html;
    index index.html;
    server_name _;
    location / {{
        try_files $uri $uri/ =404;
    }}
}}
"""
        self.write_config("/etc/nginx/sites-available/default", config)

        # Enable and start
        self.enable_service("nginx")
        self.restart_service("nginx")
        self.log.info(f"Web server listening on port {http_port}")


run(%s)
`, className, className, className)
}

func databaseManifest(id, name string) string {
	return fmt.Sprintf(`id: %s
name: "%s"
description: "TODO: Describe your database app"
version: "0.1.0"
categories:
  - database
tags:
  - postgresql
  - database
maintainers:
  - "Your Name"

lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    unprivileged: true
    cores: 2
    memory_mb: 1024
    disk_gb: 8
    onboot: true

inputs:
  - key: db_name
    label: "Database Name"
    type: string
    default: "mydb"
    required: true
    help: "Name of the database to create"
  - key: db_user
    label: "Database User"
    type: string
    default: "dbuser"
    required: true
  - key: db_password
    label: "Database Password"
    type: secret
    required: true
    help: "Password for the database user"
  - key: db_port
    label: "Database Port"
    type: number
    default: 5432
    required: true
    validation:
      min: 1
      max: 65535

provisioning:
  script: provision/install.py
  timeout_sec: 600
  redact_keys:
    - db_password

permissions:
  packages:
    - postgresql
    - postgresql-client
  services:
    - postgresql
  users:
    - postgres
  paths:
    - /etc/postgresql

outputs:
  - key: connection_string
    label: "Connection String"
    value: "postgresql://{{db_user}}:***@{{IP}}:{{db_port}}/{{db_name}}"
  - key: port
    label: "Port"
    value: "{{db_port}}"
`, id, name)
}

func databaseScript(className string) string {
	return fmt.Sprintf(`#!/usr/bin/env python3
"""Provisioning script for %s."""
from appstore import BaseApp, run


class %s(BaseApp):
    def install(self):
        db_name = self.inputs.string("db_name", "mydb")
        db_user = self.inputs.string("db_user", "dbuser")
        db_password = self.inputs.secret("db_password")
        db_port = self.inputs.integer("db_port", 5432)

        # Install PostgreSQL
        self.apt_install(["postgresql", "postgresql-client"])

        # Configure port
        self.run_command(
            f"sed -i 's/^port = .*/port = {db_port}/' /etc/postgresql/*/main/postgresql.conf"
        )

        # Allow connections from any address
        self.run_command(
            "sed -i \"s/^#listen_addresses = .*/listen_addresses = '*'/\" /etc/postgresql/*/main/postgresql.conf"
        )

        # Restart to apply config
        self.restart_service("postgresql")

        # Create user and database
        self.run_command(
            f"su - postgres -c \"psql -c \\\"CREATE USER {db_user} WITH PASSWORD '{db_password}';\\\"\"",
        )
        self.run_command(
            f"su - postgres -c \"psql -c \\\"CREATE DATABASE {db_name} OWNER {db_user};\\\"\"",
        )

        self.log.info(f"Database '{db_name}' created on port {db_port}")


run(%s)
`, className, className, className)
}

func pythonAppManifest(id, name string) string {
	return fmt.Sprintf(`id: %s
name: "%s"
description: "TODO: Describe your Python app"
version: "0.1.0"
categories:
  - development
tags:
  - python
maintainers:
  - "Your Name"

lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    unprivileged: true
    cores: 2
    memory_mb: 1024
    disk_gb: 8
    onboot: true

inputs:
  - key: app_port
    label: "Application Port"
    type: number
    default: 8000
    required: true
    validation:
      min: 1024
      max: 65535

provisioning:
  script: provision/install.py
  timeout_sec: 600

permissions:
  packages:
    - python3
    - python3-pip
    - python3-venv
  services:
    - %s
  paths:
    - /opt/%s
    - /etc/systemd/system

outputs:
  - key: url
    label: "Application URL"
    value: "http://{{IP}}:{{app_port}}"
`, id, name, id, id)
}

func pythonAppScript(className string) string {
	return fmt.Sprintf(`#!/usr/bin/env python3
"""Provisioning script for %s."""
from appstore import BaseApp, run


class %s(BaseApp):
    def install(self):
        app_port = self.inputs.integer("app_port", 8000)

        # Install Python and create venv
        self.pkg_install("python3", "python3-pip", "python3-venv")
        self.create_venv("/opt/app/venv")

        # Install app dependencies
        # self.pip_install("flask", venv="/opt/app/venv")

        # Create and start systemd service
        self.create_service(
            "app",
            exec_start=f"/opt/app/venv/bin/python -m http.server {app_port}",
            description="%s",
            working_directory="/opt/app",
        )
        self.log.info(f"Python app running on port {app_port}")


run(%s)
`, className, className, className, className)
}

func dockerImportManifest(id, name string) string {
	return fmt.Sprintf(`id: %s
name: "%s"
description: "TODO: Describe the app being ported from Docker"
version: "0.1.0"
categories:
  - utilities
tags:
  - docker-import
maintainers:
  - "Your Name"

# NOTE: This is a Docker-to-LXC import scaffold.
# Docker containers run a single process; LXC containers run a full init system.
# You'll need to adapt the Dockerfile logic into native package installs and service configs.

lxc:
  ostemplate: "local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst"
  defaults:
    unprivileged: true
    cores: 2
    memory_mb: 1024
    disk_gb: 8
    onboot: true

# TODO: Map Docker ENV vars to inputs
inputs: []

provisioning:
  script: provision/install.py
  timeout_sec: 600

# TODO: Map Docker EXPOSE to outputs
outputs:
  - key: url
    label: "Access URL"
    value: "http://{{IP}}/"
`, id, name)
}

func dockerImportScript(className string) string {
	return fmt.Sprintf(`#!/usr/bin/env python3
"""
Docker-to-LXC import scaffold for %s.

Porting from Docker to LXC means replacing the Docker runtime with native services:
- Dockerfile RUN commands → apt_install() and run_command()
- Dockerfile COPY/ADD → write_config() or push files via assets
- Dockerfile ENV → self.inputs.string() / self.inputs.integer()
- Dockerfile EXPOSE → informational, add to outputs in app.yml
- Dockerfile ENTRYPOINT/CMD → systemd service unit
- Docker volumes → LXC bind mounts (configure in app.yml volumes section)
"""
from appstore import BaseApp, run


class %s(BaseApp):
    def install(self):
        # TODO: Replace Docker commands with LXC provisioning
        #
        # Example Docker → LXC mapping:
        #   FROM debian:12        → lxc.ostemplate in app.yml
        #   RUN apt-get install   → self.apt_install(["pkg1", "pkg2"])
        #   COPY config.conf      → self.write_config("/path/to/config", content)
        #   ENV MY_VAR=value      → self.inputs.string("my_var", "value")
        #   EXPOSE 8080           → outputs in app.yml
        #   CMD ["./app"]         → systemd service unit

        self.log.info("Docker import scaffold - implement your provisioning logic")


run(%s)
`, className, className, className)
}
