import os
from qa.tests.identity_api.conftest import lab

__all__ = ["lab"]

def pytest_collection_modifyitems(items):
    prefix = {"m14cp01": "M14-CP01-", "m14cp02": "M14-CP02-", "m14cp03": "M14-CP03-"}[os.environ["QA_CHECKPOINT"]]
    items[:] = [item for item in items if item.get_closest_marker("case") and item.get_closest_marker("case").args[0].startswith(prefix)]
