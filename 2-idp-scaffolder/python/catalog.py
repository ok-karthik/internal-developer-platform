"""Loads and validates 1-platform-catalog/catalog.yaml.

This is the Python twin of internal/catalog/catalog.go. It is the *data* layer:
it knows what the platform offers (golden paths, capabilities, output paths) and
nothing about rendering, files, or the CLI.

Run it directly to see it work:

    uv run python catalog.py
"""

from __future__ import annotations
# ^ Lets you write `GoldenPath | None` in annotations on older Pythons. Harmless
#   on 3.14 where it is already the default, and a habit worth having.

from pathlib import Path

import yaml
from pydantic import BaseModel, Field, model_validator

# NOTE: `yaml` (PyYAML) currently arrives as a transitive dependency of copier.
# When copier is removed, add `pyyaml` to pyproject.toml explicitly — depending on
# a package you never declared is a bug waiting for someone else's upgrade.


# Every destinations key the renderer will look up. Missing any of them is a
# catalog authoring error, so we fail at load rather than halfway through writing
# files to disk.
#
# IMPORTANT: this list must stay textually identical to `requiredDestinations` in
# ../golang/internal/catalog/catalog.go. If the two drift, one engine accepts a
# catalog the other rejects, and the "one catalog, two engines" claim is dead.
REQUIRED_DESTINATIONS: list[str] = [
    "per-team/apps",
    "per-team/infra",
    "per-team/gitops",
    "per-service/apps/runtimes",
    "per-service/apps/service-meta",
    "per-service/infra/capabilities",
    "per-service/gitops/capabilities",
    "per-service/gitops/release",
]

VALID_PROVISIONERS = {"terraform", "ack"}


class Capability(BaseModel):
    """A friendly name bound to a version-pinned Terraform module (or an ACK CRD).

    Subclassing BaseModel (rather than using a plain class or @dataclass) means
    pydantic checks the types when the object is *constructed*. If catalog.yaml
    had `version: 1.0` (a float, no quotes) instead of `version: "v1.0.2"`,
    you find out here — with the field name in the error — rather than three
    files later when a Terraform `?ref=` comes out malformed.

    provisioner decides which of the two per-service capability directories
    cli.py looks in: "terraform" -> per-service/infra/capabilities/*.tf.tmpl,
    "ack" -> per-service/gitops/capabilities/*.yaml.tmpl (Phase 3.9 — mirrors
    Go's internal/catalog.Capability). kind/group are only meaningful for
    provisioner: ack and are not injected into any template.
    """

    module: str
    version: str
    provisioner: str
    kind: str = ""
    group: str = ""


class GoldenPath(BaseModel):
    """One paved road: a runtime plus the capabilities that come with it."""

    name: str
    runtime: str

    # `= ""` and `= []` make these optional with a default. Note the list default
    # is safe here in a way it would NOT be in a plain function signature —
    # pydantic deep-copies defaults per instance, so there is no shared-mutable-
    # default trap. (`def f(x=[])` in normal Python is the classic bug.)
    description: str = ""
    capabilities: list[str] = []
    delivery: str = ""


class Catalog(BaseModel):
    """The whole catalog.yaml file."""

    capabilities_source_base: str

    # The YAML key is `golden-paths`, with a hyphen — not a legal Python
    # identifier, so we cannot name the attribute that. `alias` bridges the two:
    # pydantic reads "golden-paths" from the YAML, we write `.golden_paths` in code.
    golden_paths: list[GoldenPath] = Field(alias="golden-paths")

    # A dict, not a list, because capabilities are *looked up by name*
    # ("what module is postgres?"). Golden paths are a list because they are
    # *iterated* ("show me every paved road"). Pick the shape that matches how
    # the data is read; the YAML follows from that, not the other way round.
    capabilities: dict[str, Capability]

    destinations: dict[str, str]

    # Lets you build a Catalog in tests using the Python name (`golden_paths=...`)
    # as well as the YAML alias. Without this, only the alias would work.
    model_config = {"populate_by_name": True}

    @model_validator(mode="after")
    def _check(self) -> "Catalog":
        """Cross-field checks that no single field can do on its own.

        `mode="after"` means this runs once every individual field has already
        been parsed and type-checked, so `self` is fully built and safe to read.
        Raising ValueError here turns into a pydantic ValidationError for the caller.
        """
        if not self.capabilities_source_base:
            return _fail("capabilities_source_base is empty")

        # 1. Every key the renderer will ask for must exist.
        for key in REQUIRED_DESTINATIONS:
            if key not in self.destinations:
                return _fail(f"missing destination key: {key}")

        # 1b. Every capability must declare a provisioner cli.py knows how to
        # dispatch on — fail here, not as an unreadable "template not found"
        # three layers into add-service.
        for name, cap in self.capabilities.items():
            if cap.provisioner not in VALID_PROVISIONERS:
                return _fail(
                    f"capability {name!r} has unknown provisioner {cap.provisioner!r} "
                    f"(must be one of: {', '.join(sorted(VALID_PROVISIONERS))})"
                )

        # 2. A golden path may only reference capabilities that actually exist,
        #    otherwise the failure surfaces at render time as a missing template.
        for gp in self.golden_paths:
            if not gp.runtime:
                return _fail(f"golden-path {gp.name!r} has no runtime")
            for cap in gp.capabilities:
                if cap not in self.capabilities:
                    return _fail(
                        f"golden-path {gp.name!r} references unknown capability {cap!r} "
                        f"(available: {', '.join(sorted(self.capabilities))})"
                    )

        return self  # `mode="after"` validators must return the model.


def _fail(message: str):
    """Raise a ValueError. Exists only so the validator above reads as a list of
    checks rather than a wall of `raise` statements."""
    raise ValueError(message)


def load_catalog(path: Path) -> Catalog:
    """Read catalog.yaml from disk, parse it, and validate it.

    Validating *here*, at the load boundary, is the whole point: the error can
    still name the offending key, and nothing has been written yet. The
    alternative — discovering a missing destination mid-render — leaves half a
    tree on disk and an error that points at the wrong place.
    """
    raw = yaml.safe_load(path.read_text(encoding="utf-8"))
    # safe_load, never plain load: `load` can construct arbitrary Python objects
    # from YAML tags, which is a remote-code-execution hole if the file is ever
    # fetched rather than local.

    if not isinstance(raw, dict):
        raise ValueError(f"{path} did not parse to a mapping (got {type(raw).__name__})")

    # `**raw` spreads the dict into keyword arguments. Every validator above runs
    # right here; if anything is wrong, this line raises.
    return Catalog(**raw)


def find_golden_path(catalog: Catalog, name: str) -> GoldenPath | None:
    """Look up one golden path by name, or None if there is no such path.

    Returning `| None` is the Python equivalent of Go's `(*GoldenPath, bool)`.
    The caller is forced to deal with the miss:

        gp = find_golden_path(catalog, "go-service-postgres")
        if gp is None:
            ...
    """
    for gp in catalog.golden_paths:
        if gp.name == name:
            return gp
    return None


if __name__ == "__main__":
    # Runs only when you execute this file directly (`python catalog.py`), not
    # when another module imports it. Handy as a smoke test while building.
    here = Path(__file__).resolve().parent
    catalog_file = here.parent.parent / "1-platform-catalog" / "catalog.yaml"

    catalog = load_catalog(catalog_file)

    print(f"loaded {catalog_file}")
    print(f"  source base : {catalog.capabilities_source_base[:60]}...")
    print(f"  capabilities: {', '.join(sorted(catalog.capabilities))}")
    print(f"  destinations: {len(catalog.destinations)} keys")
    print("  golden paths:")
    for gp in catalog.golden_paths:
        print(f"    - {gp.name:24} runtime={gp.runtime:8} caps={gp.capabilities}")

    # Prove the lookup works, both ways.
    found = find_golden_path(catalog, "go-service-postgres")
    print(f"\n  lookup 'go-service-postgres' -> {found.runtime if found else None}")
    print(f"  lookup 'nope'                -> {find_golden_path(catalog, 'nope')}")

    # Prove a capability resolves to a real, version-pinned module.
    if found and found.capabilities:
        cap = catalog.capabilities[found.capabilities[0]]
        print(f"\n  {found.capabilities[0]} -> {cap.module}?ref={cap.version}")
