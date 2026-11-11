import os

from qa.tests.identity_api.conftest import lab

__all__ = ["lab"]


def pytest_collection_modifyitems(items):
    assert os.environ["QA_CHECKPOINT"] == "m05cp05"
    items[:] = [item for item in items if item.get_closest_marker("case") and item.get_closest_marker("case").args[0].startswith("M05-CP05-")]
