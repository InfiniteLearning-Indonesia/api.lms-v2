from qa.tests.identity_api.conftest import lab

__all__ = ["lab"]


def pytest_collection_modifyitems(items):
    import os
    checkpoint = os.environ.get("QA_CHECKPOINT")
    prefix = {"m15cp01": "M15-CP01-", "m15cp02": "M15-CP02-", "m15cp03": "M15-CP03-", "m15cp04": "M15-CP04-", "m15cp05": "M15-CP05-"}.get(checkpoint, "UNREGISTERED-")
    items[:] = [item for item in items if item.get_closest_marker("case") and item.get_closest_marker("case").args[0].startswith(prefix)]
