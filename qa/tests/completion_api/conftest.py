from qa.tests.identity_api.conftest import lab
import os

__all__ = ["lab"]


def pytest_collection_modifyitems(items):
    checkpoint = os.environ.get("QA_CHECKPOINT", "m10cp01").upper().replace("M10CP", "M10-CP")
    items[:] = [
        item for item in items
        if item.get_closest_marker("case")
        and item.get_closest_marker("case").args[0].startswith(checkpoint + "-")
    ]
