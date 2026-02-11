"""OCI registry client for pulling binaries from Docker images.

Downloads a binary from a Docker/OCI image without needing Docker installed.
Supports Docker Hub with token authentication and multi-arch manifest resolution.
"""

import io
import json
import os
import platform
import tarfile
import urllib.request


class OCIClient:
    """Pulls files from OCI/Docker images via the registry HTTP API."""

    DOCKER_AUTH_URL = "https://auth.docker.io/token"
    DOCKER_REGISTRY_URL = "https://registry-1.docker.io"

    ACCEPT_TYPES = ", ".join([
        "application/vnd.oci.image.index.v1+json",
        "application/vnd.oci.image.manifest.v1+json",
        "application/vnd.docker.distribution.manifest.list.v2+json",
        "application/vnd.docker.distribution.manifest.v2+json",
    ])

    def __init__(self, log=None):
        self.log = log

    def _info(self, msg):
        if self.log:
            self.log.info(msg)

    def _request_json(self, url, token=None):
        """Make an HTTP request returning parsed JSON."""
        req = urllib.request.Request(url)
        if token:
            req.add_header("Authorization", f"Bearer {token}")
        req.add_header("Accept", self.ACCEPT_TYPES)
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read())

    def _download_bytes(self, url, token=None):
        """Download raw bytes from a URL."""
        req = urllib.request.Request(url)
        if token:
            req.add_header("Authorization", f"Bearer {token}")
        with urllib.request.urlopen(req, timeout=120) as resp:
            return resp.read()

    def _get_token(self, image):
        """Get a Docker Hub authentication token for the given image."""
        url = (
            f"{self.DOCKER_AUTH_URL}"
            f"?service=registry.docker.io"
            f"&scope=repository:{image}:pull"
        )
        data = self._request_json(url)
        return data["token"]

    def _detect_arch(self):
        """Detect current platform architecture."""
        machine = platform.machine()
        if machine in ("x86_64", "AMD64"):
            return "amd64"
        if machine in ("aarch64", "arm64"):
            return "arm64"
        raise RuntimeError(f"Unsupported architecture: {machine}")

    def pull_binary(self, image, dest, tag="latest", binary_names=None):
        """Download a binary from a Docker/OCI image.

        Args:
            image: Docker image name (e.g., "qmcgaw/gluetun").
            dest: Destination path for the extracted binary.
            tag: Image tag (default "latest").
            binary_names: List of filenames to look for in layers.
                         If None, uses the basename of dest.
        """
        arch = self._detect_arch()
        if binary_names is None:
            base = os.path.basename(dest).lstrip("-")
            binary_names = [base, f"{base}-entrypoint"]

        self._info(f"Pulling binary for {arch} from {image}:{tag}...")

        # Step 1: Auth
        token = self._get_token(image)

        # Step 2: Fetch manifest index
        manifest_url = (
            f"{self.DOCKER_REGISTRY_URL}/v2/{image}/manifests/{tag}"
        )
        index = self._request_json(manifest_url, token)

        # Step 3: Find arch-specific manifest
        arch_digest = None
        for m in index.get("manifests", []):
            p = m.get("platform", {})
            if p.get("architecture") == arch and p.get("os") == "linux":
                arch_digest = m["digest"]
                break

        if not arch_digest:
            raise RuntimeError(
                f"No linux/{arch} manifest found in {image}:{tag}"
            )

        self._info(f"Found {arch} manifest: {arch_digest[:20]}...")

        # Step 4: Fetch image manifest
        img_url = (
            f"{self.DOCKER_REGISTRY_URL}/v2/{image}/manifests/{arch_digest}"
        )
        img_manifest = self._request_json(img_url, token)

        # Step 5: Download layers and extract binary
        layers = img_manifest.get("layers", [])
        self._info(f"Scanning {len(layers)} layers for binary...")

        for i, layer in enumerate(layers):
            digest = layer["digest"]
            blob_url = (
                f"{self.DOCKER_REGISTRY_URL}/v2/{image}/blobs/{digest}"
            )
            self._info(
                f"Downloading layer {i+1}/{len(layers)} ({digest[:20]}...)..."
            )
            blob = self._download_bytes(blob_url, token)

            try:
                with tarfile.open(fileobj=io.BytesIO(blob), mode="r:*") as tar:
                    for member in tar.getmembers():
                        name = member.name.lstrip("./")
                        if name in binary_names and member.isfile():
                            self._info(
                                f"Found binary '{name}' in layer {i+1} "
                                f"({member.size // 1024 // 1024}MB)"
                            )
                            f = tar.extractfile(member)
                            data = f.read()
                            os.makedirs(
                                os.path.dirname(dest) or ".", exist_ok=True
                            )
                            with open(dest, "wb") as out:
                                out.write(data)
                            os.chmod(dest, 0o755)
                            self._info(f"Installed binary at {dest}")
                            return
            except (tarfile.TarError, EOFError):
                continue

        raise RuntimeError(
            f"Binary {binary_names} not found in any layer of {image}:{tag}"
        )
