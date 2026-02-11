# GitLab CE

Self-hosted Git repository management, CI/CD pipelines, issue tracking, and DevOps platform running in a Proxmox LXC container.

## What's Included

The GitLab omnibus package bundles everything into a single install:

- **Git hosting** — repositories, merge requests, code review
- **CI/CD** — built-in pipelines with `.gitlab-ci.yml`
- **Issue tracking** — boards, milestones, labels
- **Container Registry** — optional built-in Docker/OCI registry
- **GitLab Pages** — optional static site hosting
- **Wiki** — per-project documentation

## Requirements

- **Minimum:** 4 cores, 8 GB RAM, 20 GB disk
- **Recommended:** 4+ cores, 16 GB RAM, 50 GB disk (for teams > 20 users)
- Ubuntu 24.04 LXC container (unprivileged, nesting enabled)

## Configuration

All settings marked as "reconfigurable" can be changed after install without rebuilding the container. The app uses a template-based configuration system (`gitlab.rb.tmpl`) that renders to `/etc/gitlab/gitlab.rb` and runs `gitlab-ctl reconfigure`.

### Inputs

| Input | Default | Reconfigurable | Description |
|-------|---------|---------------|-------------|
| External URL | auto-detect | Yes | URL users access GitLab at |
| HTTP Port | 80 | Yes | Web interface port |
| SSH Port | 22 | Yes | Git SSH port |
| Container Registry | off | Yes | Enable OCI registry |
| GitLab Pages | off | Yes | Enable static site hosting |
| Email Confirmation | off | Yes | Require email verification for new accounts |
| Root Password | random | No | Initial admin password (min 8 chars) |

## Post-Install

- Access the web UI at the URL shown in outputs
- Log in as `root` with the password you set (or check outputs for the generated one)
- First request after install may take 30-60 seconds while Puma starts up
