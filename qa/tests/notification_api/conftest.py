from qa.tests.identity_api.conftest import lab

__all__ = ["lab"]


def pytest_collection_modifyitems(items):
    import os
    prefixes = {"m13cp01": "M13-CP01-", "m13cp02": "M13-CP02-"}
    prefix = prefixes[os.environ["QA_CHECKPOINT"]]
    items[:] = [item for item in items if item.get_closest_marker("case") and item.get_closest_marker("case").args[0].startswith(prefix)]
