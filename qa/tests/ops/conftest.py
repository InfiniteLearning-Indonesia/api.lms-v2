"""Small retained synthetic fixtures under the run's admitted artifact root.

Use inherited directory permissions rather than pytest's platform-specific
private temp-root ACL rewrite. No ACL changes or existing-directory cleanup.
This fixture is not a protected destination for real baseline data.
"""
import os
from pathlib import Path
import uuid

import pytest

from qa import storage


@pytest.fixture
def qa_scratch():
    root = Path(os.environ["QA_ARTIFACT_ROOT"]).resolve(strict=True)
    if storage.reparse(root):
        raise ValueError("LINKED_FIXTURE_ROOT")
    path = root / ("ops-fixture-" + uuid.uuid4().hex)
    path.mkdir()
    return path
