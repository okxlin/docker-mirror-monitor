#!/usr/bin/env python3
"""Bind every release platform's Trivy report to the once-built image index."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

PLATFORMS = {"linux/amd64", "linux/arm64", "linux/arm/v7", "linux/ppc64le", "linux/s390x"}
DIGEST = re.compile(r"sha256:[a-f0-9]{64}")
INDEX_TYPES = {"application/vnd.oci.image.index.v1+json",
               "application/vnd.docker.distribution.manifest.list.v2+json"}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def check_index(raw, digest):
    require(DIGEST.fullmatch(digest), "invalid expected image digest")
    require("sha256:" + hashlib.sha256(raw).hexdigest() == digest, "image index digest mismatch")
    index = json.loads(raw)
    require(isinstance(index, dict), "image index must be an object")
    require(index.get("schemaVersion") == 2 and index.get("mediaType") in INDEX_TYPES,
            "expected a multi-platform image index")
    platforms = {}
    for descriptor in index.get("manifests", []):
        platform = descriptor.get("platform", {})
        os, arch = platform.get("os"), platform.get("architecture")
        if os == arch == "unknown" and descriptor.get("annotations", {}).get(
                "vnd.docker.reference.type") == "attestation-manifest":
            continue
        name = f"{os}/{arch}"
        if arch == "arm":
            name += "/" + platform.get("variant", "")
        require(name in PLATFORMS and name not in platforms, "unexpected or duplicate image platform")
        require(DIGEST.fullmatch(descriptor.get("digest", "")), "invalid platform manifest digest")
        platforms[name] = descriptor["digest"]
    require(platforms.keys() == PLATFORMS, "release image is missing a required platform")
    return platforms


def check_report(inputs, report):
    require(isinstance(inputs, dict) and isinstance(report, dict), "scan inputs and report must be objects")
    platform = inputs.get("platform")
    require(platform in PLATFORMS and DIGEST.fullmatch(inputs.get("config_digest", "")),
            "invalid scan inputs")
    require(report.get("SchemaVersion") == 2 and report.get("ArtifactType") == "container_image",
            "expected a container image vulnerability report")
    metadata = report.get("Metadata", {})
    require(metadata.get("ImageID") == inputs["config_digest"], "scan image config digest mismatch")
    config = metadata.get("ImageConfig", {})
    os, arch, *_ = platform.split("/")
    require(config.get("os") == os and config.get("architecture") == arch,
            "scan platform mismatch")
    results = report.get("Results", [])
    require(any(row.get("Class") == "os-pkgs" for row in results), "OS package scan is missing")
    require(any(row.get("Type") == "gobinary" and row.get("Target", "").endswith(
                "/docker-mirror-monitor") for row in results), "monitor binary scan is missing")
    require(all(not row.get("Vulnerabilities") for row in results), "image vulnerabilities remain")


def inspect(reference):
    require(re.fullmatch(r"(?:ghcr\.io|docker\.io)/[a-z0-9]+(?:[._-][a-z0-9]+)*/docker-mirror-monitor"
                        r"(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(?:@sha256:[a-f0-9]{64})?", reference),
            "unexpected release image reference")
    result = subprocess.run(["docker", "buildx", "imagetools", "inspect", reference, "--raw"],
                            capture_output=True, check=True, timeout=120)
    require(len(result.stdout) < 4 * 1024 * 1024, "image manifest exceeds size limit")
    return result.stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for command in ("index", "resolve"):
        sub = commands.add_parser(command)
        sub.add_argument("--image", required=True)
        sub.add_argument("--digest", required=True)
        if command == "resolve":
            sub.add_argument("--platform", choices=sorted(PLATFORMS), required=True)
            sub.add_argument("--output", type=Path, required=True)
    report = commands.add_parser("report")
    report.add_argument("--inputs", type=Path, required=True)
    report.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "report":
        inputs = json.loads(args.inputs.read_text(encoding="utf-8"))
        check_report(inputs, json.loads(args.report.read_text(encoding="utf-8")))
        print(f"Verified zero-vulnerability {inputs['platform']} image {inputs['config_digest']}")
        return
    platforms = check_index(inspect(args.image), args.digest)
    if args.command == "index":
        print(f"Verified all five release platforms in {args.digest}")
        return
    require(args.image.endswith("@" + args.digest), "scan image must use the built index digest")
    raw = inspect(args.image.partition("@")[0] + "@" + platforms[args.platform])
    require("sha256:" + hashlib.sha256(raw).hexdigest() == platforms[args.platform],
            "platform manifest digest mismatch")
    config_digest = json.loads(raw).get("config", {}).get("digest", "")
    require(DIGEST.fullmatch(config_digest), "invalid image config digest")
    with args.output.open("x", encoding="utf-8") as output:
        output.write(json.dumps({"platform": args.platform, "index_digest": args.digest,
                                 "config_digest": config_digest}, indent=2) + "\n")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(f"ERROR: release image gate failed: {error}", file=sys.stderr)
        sys.exit(1)
