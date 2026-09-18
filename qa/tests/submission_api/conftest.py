from qa.tests.identity_api.conftest import lab

__all__ = ["lab"]


def pytest_collection_modifyitems(items):
    import os
    checkpoint = os.environ.get("QA_CHECKPOINT")
    prefix = {
        "m08cp01": "M08-CP01-", "m08cp02": "M08-CP02-",
        "m08cp03": "M08-CP03-", "m08cp04": "M08-CP04-",
    }.get(checkpoint, "UNREGISTERED-")
    items[:] = [item for item in items if (item.get_closest_marker("case") and item.get_closest_marker("case").args[0].startswith(prefix))]
