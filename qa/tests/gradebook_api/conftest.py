from qa.tests.identity_api.conftest import lab

__all__ = ["lab"]


def pytest_collection_modifyitems(items):
    import os
    prefix = {"m09cp01": "M09-CP01-", "m09cp02": "M09-CP02-"}[os.environ["QA_CHECKPOINT"]]
    items[:] = [item for item in items if item.get_closest_marker("case") and item.get_closest_marker("case").args[0].startswith(prefix)]
