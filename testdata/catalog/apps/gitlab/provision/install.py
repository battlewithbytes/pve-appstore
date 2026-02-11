#!/usr/bin/env python3
"""GitLab CE — self-hosted DevOps platform."""
import os
from urllib.parse import urlparse
from appstore import BaseApp, run

GITLAB_RB = "/etc/gitlab/gitlab.rb"


class GitLabApp(BaseApp):
    def install(self):
        # Prerequisites — locales required for PostgreSQL initdb
        self.pkg_install("curl", "openssh-server", "ca-certificates", "tzdata", "perl", "locales")
        self.log.info("Generating en_US.UTF-8 locale...")
        self.run_command(["locale-gen", "en_US.UTF-8"])
        self.run_command(["update-locale", "LANG=en_US.UTF-8"])

        # Add GitLab apt repository
        self.log.info("Adding GitLab package repository...")
        self.add_apt_repository(
            "https://packages.gitlab.com/gitlab/gitlab-ce/ubuntu",
            key_url="https://packages.gitlab.com/gitlab/gitlab-ce/gpgkey",
            name="gitlab-ce",
        )

        external_url = self.inputs.string("external_url", "")
        if not external_url:
            external_url = f"http://{os.environ.get('CONTAINER_IP', 'localhost')}"

        # EXTERNAL_URL is needed by the gitlab-ce package during install.
        # GITLAB_SKIP_RECONFIGURE is set in provisioning.env in app.yml to skip
        # the silent 10-15 min postinst reconfigure — our configure() handles it instead.
        self.log.info("Installing GitLab CE package (this downloads ~1 GB)...")
        os.environ["EXTERNAL_URL"] = external_url
        self.pkg_install("gitlab-ce")

        # Write configuration and reconfigure
        self.configure()

        # Initial root password
        initial_password = self.inputs.string("initial_root_password", "")
        if initial_password:
            self.log.info("Setting initial root password...")
            self.run_command([
                "gitlab-rake", "gitlab:password:reset",
            ], input_text=f"root\n{initial_password}\n{initial_password}\n", check=False)

        self.log.info("GitLab CE installed successfully")

    def configure(self):
        """Write gitlab.rb from template and reconfigure. Called by install() and reconfigure."""
        external_url = self.inputs.string("external_url", "")
        gitlab_port = self.inputs.integer("gitlab_port", 80)
        ssh_port = self.inputs.integer("ssh_port", 22)
        registry_enabled = self.inputs.boolean("registry_enabled", False)
        pages_enabled = self.inputs.boolean("pages_enabled", False)
        require_email = self.inputs.boolean("require_email_confirmation", False)

        if not external_url:
            external_url = f"http://{os.environ.get('CONTAINER_IP', 'localhost')}"

        # If port is non-standard, append to URL
        if gitlab_port != 80 and ":" not in external_url.split("//")[-1]:
            external_url = f"{external_url}:{gitlab_port}"

        hostname = urlparse(external_url).hostname or "localhost"

        self.render_template("gitlab.rb.tmpl", GITLAB_RB,
            external_url=external_url,
            gitlab_port=gitlab_port,
            ssh_port=ssh_port,
            hostname=hostname,
            registry_enabled=registry_enabled,
            pages_enabled=pages_enabled,
            send_confirmation_email="true" if require_email else "false",
        )

        # Reconfigure GitLab to apply changes
        self.log.info("Running gitlab-ctl reconfigure (this may take a few minutes)...")
        self.run_command(["gitlab-ctl", "reconfigure"])

        # Apply settings to database — gitlab.rb values are only initial defaults,
        # after first reconfigure the database takes precedence.
        email_setting = "hard" if require_email else "off"
        self.log.info("Applying sign-up settings to database...")
        self.run_command(["gitlab-rails", "runner",
            "ApplicationSetting.current.update!("
            "require_admin_approval_after_user_signup: false, "
            f"email_confirmation_setting: '{email_setting}'"
            ")"
        ], check=False)
        self.log.info("GitLab reconfigured successfully")


run(GitLabApp)
