from qa.tests.identity_api.conftest import lab

__all__ = ["lab"]


def pytest_collection_modifyitems(items):
    import os
    prefixes = {"m12cp01": "M12-CP01-", "m12cp02": "M12-CP02-", "m12cp03": "M12-CP03-"}
    prefix = prefixes[os.environ["QA_CHECKPOINT"]]
    items[:] = [item for item in items if item.get_closest_marker("case") and item.get_closest_marker("case").args[0].startswith(prefix)]
