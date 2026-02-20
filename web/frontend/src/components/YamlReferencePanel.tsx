const yamlReference = [
  { group: 'Top-Level (required)', fields: [
    { name: 'id', type: 'string', desc: 'Unique kebab-case identifier (e.g. my-app)' },
    { name: 'name', type: 'string', desc: 'Display name shown in the catalog' },
    { name: 'description', type: 'string', desc: 'Short one-line description' },
    { name: 'version', type: 'string', desc: 'App version (e.g. "1.0.0")' },
    { name: 'categories', type: 'string[]', desc: 'At least one: development, media, network, etc.' },
  ]},
  { group: 'Top-Level (optional)', fields: [
    { name: 'overview', type: 'string', desc: 'Multi-line markdown overview (use | for block)' },
    { name: 'tags', type: 'string[]', desc: 'Search tags (e.g. git, ci-cd, docker)' },
    { name: 'homepage', type: 'string', desc: 'URL to project homepage' },
    { name: 'license', type: 'string', desc: 'License identifier (e.g. MIT, GPL-3.0)' },
    { name: 'official', type: 'bool', desc: 'Maintained by the app store team' },
    { name: 'featured', type: 'bool', desc: 'Show in featured section' },
    { name: 'maintainers', type: 'string[]', desc: 'List of maintainer names' },
  ]},
  { group: 'lxc (required)', fields: [
    { name: 'ostemplate', type: 'string', desc: 'Template name (e.g. ubuntu-24.04, debian-12, alpine-3.21)' },
    { name: 'defaults.unprivileged', type: 'bool', desc: 'true recommended; false for special needs' },
    { name: 'defaults.cores', type: 'int', desc: 'CPU cores (min 1)' },
    { name: 'defaults.memory_mb', type: 'int', desc: 'RAM in MB (min 128)' },
    { name: 'defaults.disk_gb', type: 'int', desc: 'Root disk in GB (min 1)' },
    { name: 'defaults.features', type: 'string[]', desc: 'LXC features: nesting, keyctl, fuse' },
    { name: 'defaults.onboot', type: 'bool', desc: 'Start container on host boot' },
    { name: 'defaults.require_static_ip', type: 'bool', desc: 'Require static IP at install time (e.g. DNS servers)' },
    { name: 'extra_config', type: 'string[]', desc: 'Raw lxc.* config lines appended to CT conf' },
  ]},
  { group: 'inputs[]', fields: [
    { name: 'key', type: 'string', desc: 'Unique key used in install.py via self.inputs' },
    { name: 'label', type: 'string', desc: 'Display label in the install form' },
    { name: 'type', type: 'string', desc: 'string | number | boolean | secret | select' },
    { name: 'default', type: 'any', desc: 'Default value (type-appropriate)' },
    { name: 'required', type: 'bool', desc: 'Must be provided at install time' },
    { name: 'reconfigurable', type: 'bool', desc: 'Can be changed post-install via reconfigure' },
    { name: 'group', type: 'string', desc: 'Group inputs visually (e.g. General, Network)' },
    { name: 'description', type: 'string', desc: 'Short description below the input' },
    { name: 'help', type: 'string', desc: 'Tooltip/help text' },
    { name: 'validation.regex', type: 'string', desc: 'Regex pattern to validate string input' },
    { name: 'validation.min / max', type: 'number', desc: 'Min/max for number inputs' },
    { name: 'validation.min_length / max_length', type: 'int', desc: 'String length constraints' },
    { name: 'validation.enum', type: 'string[]', desc: 'Allowed values for select type' },
    { name: 'validation.format', type: 'string', desc: 'Built-in format: ipv4, cidr, url, email, hostname' },
    { name: 'show_when.input', type: 'string', desc: 'Key of another input to check' },
    { name: 'show_when.values', type: 'string[]', desc: 'Show only when input matches one of these' },
  ]},
  { group: 'volumes[]', fields: [
    { name: 'name', type: 'string', desc: 'Unique volume name (e.g. data, config)' },
    { name: 'type', type: 'string', desc: 'volume (managed disk) or bind (host path)' },
    { name: 'mount_path', type: 'string', desc: 'Absolute path inside the container' },
    { name: 'size_gb', type: 'int', desc: 'Size in GB (type=volume only, min 1)' },
    { name: 'label', type: 'string', desc: 'Display label in the install form' },
    { name: 'default_host_path', type: 'string', desc: 'Suggested host path (type=bind only)' },
    { name: 'required', type: 'bool', desc: 'Volume must be configured at install' },
    { name: 'read_only', type: 'bool', desc: 'Mount as read-only' },
    { name: 'description', type: 'string', desc: 'Description shown in the install form' },
  ]},
  { group: 'gpu', fields: [
    { name: 'required', type: 'bool', desc: 'true = fail if no GPU; false = optional' },
    { name: 'notes', type: 'string', desc: 'Shown to user in install form' },
    { name: 'supported', type: 'string[]', desc: '(deprecated) GPU vendors — auto-detected now' },
    { name: 'profiles', type: 'string[]', desc: '(deprecated) Device profiles — auto-detected now' },
  ]},
  { group: 'provisioning', fields: [
    { name: 'script', type: 'string', desc: 'Path to install script (e.g. provision/install.py)' },
    { name: 'timeout_sec', type: 'int', desc: 'Max provision time (default: 600)' },
    { name: 'env', type: 'map', desc: 'Extra env vars passed to the script' },
  ]},
  { group: 'permissions', fields: [
    { name: 'packages', type: 'string[]', desc: 'Allowed apt/apk packages' },
    { name: 'pip', type: 'string[]', desc: 'Allowed pip packages' },
    { name: 'urls', type: 'string[]', desc: 'Allowed URLs for downloads (supports *)' },
    { name: 'apt_repos', type: 'string[]', desc: 'Allowed APT repository URLs' },
    { name: 'installer_scripts', type: 'string[]', desc: 'Allowed installer script URLs' },
    { name: 'paths', type: 'string[]', desc: 'Allowed filesystem path prefixes' },
    { name: 'commands', type: 'string[]', desc: 'Allowed binary names (supports *)' },
    { name: 'services', type: 'string[]', desc: 'Allowed systemd service names' },
    { name: 'users', type: 'string[]', desc: 'Allowed usernames for create_user()' },
  ]},
  { group: 'outputs[]', fields: [
    { name: 'key', type: 'string', desc: 'Unique output key' },
    { name: 'label', type: 'string', desc: 'Display label (e.g. Web UI, Admin Password)' },
    { name: 'value', type: 'string', desc: 'Template: http://{{ip}}:{{port}} or {{input_key}}' },
  ]},
]

export function YamlReferencePanel() {
  return (
    <div className="border border-border rounded-lg mt-4 overflow-hidden">
      <div className="bg-bg-card px-4 py-2 border-b border-border">
        <span className="text-xs text-text-muted font-mono uppercase">app.yml Manifest Reference</span>
      </div>
      <div className="p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 max-h-[400px] overflow-y-auto">
        {yamlReference.map(group => (
          <div key={group.group}>
            <h4 className="text-xs font-mono text-primary font-bold mb-1">{group.group}</h4>
            {group.fields.map(f => (
              <div key={f.name} className="mb-1.5">
                <div className="flex items-baseline gap-1.5">
                  <code className="text-xs font-mono text-text-primary">{f.name}</code>
                  <span className="text-[10px] font-mono text-text-muted">{f.type}</span>
                </div>
                <p className="text-xs text-text-muted mt-0.5">{f.desc}</p>
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}
