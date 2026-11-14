#!/usr/bin/env python3
"""Reject incomplete or stale image verification before release promotion."""

import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location("gate", Path(__file__).with_name("release-image-gate.py"))
GATE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(GATE)


class ReleaseImageGateTests(unittest.TestCase):
    def setUp(self):
        self.index_data = {"schemaVersion": 2, "mediaType": "application/vnd.oci.image.index.v1+json",
                      "manifests": [{"digest": "sha256:" + "a" * 64,
                                     "platform": dict(zip(["os", "architecture", "variant"], p.split("/")))}
                                    for p in sorted(GATE.PLATFORMS)]}
        self.inputs = {"platform": "linux/amd64", "config_digest": "sha256:" + "b" * 64}
        self.report = {"SchemaVersion": 2, "ArtifactType": "container_image", "Metadata": {
            "ImageID": self.inputs["config_digest"], "ImageConfig": {"os": "linux", "architecture": "amd64"}},
            "Results": [{"Class": "os-pkgs", "Type": "alpine"},
                        {"Class": "lang-pkgs", "Type": "gobinary", "Target": "app/docker-mirror-monitor"}]}

    def check_index(self, data):
        raw = json.dumps(data).encode()
        return GATE.check_index(raw, "sha256:" + hashlib.sha256(raw).hexdigest())

    def test_complete_index_includes_all_five_release_platforms(self):
        self.assertEqual(self.check_index(self.index_data).keys(), GATE.PLATFORMS)

    def test_only_declared_attestations_can_have_unknown_platforms(self):
        descriptor = {"digest": "sha256:" + "c" * 64,
                      "platform": {"os": "unknown", "architecture": "unknown"},
                      "annotations": {"vnd.docker.reference.type": "attestation-manifest"}}
        self.index_data["manifests"].append(descriptor)
        self.assertEqual(self.check_index(self.index_data).keys(), GATE.PLATFORMS)
        descriptor.pop("annotations")
        with self.assertRaisesRegex(ValueError, "unexpected"):
            self.check_index(self.index_data)

    def test_missing_duplicate_and_unexpected_platforms_fail(self):
        for change in (lambda d: d["manifests"].pop(),
                       lambda d: d["manifests"].append(d["manifests"][0]),
                       lambda d: d["manifests"][0]["platform"].update(architecture="riscv64")):
            data = copy.deepcopy(self.index_data)
            change(data)
            with self.assertRaises(ValueError):
                self.check_index(data)

    def test_registry_index_must_match_the_built_digest(self):
        with self.assertRaisesRegex(ValueError, "digest mismatch"):
            GATE.check_index(json.dumps(self.index_data).encode(), "sha256:" + "f" * 64)

    def test_clean_report_matches_both_image_identity_and_platform(self):
        GATE.check_report(self.inputs, self.report)

    def test_stale_wrong_platform_missing_or_vulnerable_reports_fail(self):
        for change in (lambda d: d["Metadata"].update(ImageID="sha256:" + "f" * 64),
                       lambda d: d["Metadata"]["ImageConfig"].update(architecture="arm64"),
                       lambda d: d.update(ArtifactType="filesystem"),
                       lambda d: d["Results"].pop(0),
                       lambda d: d["Results"].pop(),
                       lambda d: d["Results"][0].update(Vulnerabilities=[{"Severity": "LOW"}])):
            report = copy.deepcopy(self.report)
            change(report)
            with self.assertRaises(ValueError):
                GATE.check_report(self.inputs, report)


if __name__ == "__main__":
    unittest.main()
