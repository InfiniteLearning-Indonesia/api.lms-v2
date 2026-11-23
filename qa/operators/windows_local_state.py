"""Read-only Windows process/listener evidence. Never signals a process."""
import ctypes
import os
import socket
import struct

from qa.operators.freeze_staging_baseline import Stop


def listener_pids_from_table(raw, port):
    if not 1 <= port <= 65535 or len(raw) < 4:
        raise Stop('WINDOWS_TCP_TABLE_INVALID')
    count = struct.unpack_from('<I', raw)[0]
    if count > 65536 or 4 + count * 24 > len(raw):
        raise Stop('WINDOWS_TCP_TABLE_INVALID')
    pids = set()
    for offset in range(4, 4 + count * 24, 24):
        state, address, encoded_port, _, _, pid = struct.unpack_from('<IIIIII', raw, offset)
        if state != 2:
            raise Stop('WINDOWS_TCP_LISTENER_STATE_INVALID')
        # DWORD holds a network-order 16-bit port. Include all IPv4 interfaces:
        # any listener at this recorded port conservatively blocks a new clone.
        if socket.ntohs(encoded_port & 65535) == port:
            pids.add(pid)
    return sorted(pids)


def windows_process_ids():
    if os.name != 'nt':
        raise Stop('WINDOWS_OWNER_PROCESS_INSPECTION_REQUIRED')
    from ctypes import wintypes as w
    api = ctypes.WinDLL('kernel32', use_last_error=True)
    class Entry(ctypes.Structure):
        _fields_ = [('size', w.DWORD), ('usage', w.DWORD), ('pid', w.DWORD),
                    ('heap', ctypes.c_size_t), ('module', w.DWORD), ('threads', w.DWORD),
                    ('parent', w.DWORD), ('priority', w.LONG), ('flags', w.DWORD),
                    ('name', w.WCHAR * 260)]
    api.CreateToolhelp32Snapshot.argtypes = [w.DWORD, w.DWORD]
    api.CreateToolhelp32Snapshot.restype = w.HANDLE
    api.Process32FirstW.argtypes = [w.HANDLE, ctypes.POINTER(Entry)]
    api.Process32FirstW.restype = w.BOOL
    api.Process32NextW.argtypes = [w.HANDLE, ctypes.POINTER(Entry)]
    api.Process32NextW.restype = w.BOOL
    api.CloseHandle.argtypes = [w.HANDLE]
    handle = api.CreateToolhelp32Snapshot(2, 0)  # TH32CS_SNAPPROCESS
    if handle in (None, 0, ctypes.c_void_p(-1).value):
        raise Stop('WINDOWS_PROCESS_SNAPSHOT_UNAVAILABLE')
    try:
        entry = Entry(); entry.size = ctypes.sizeof(entry)
        if not api.Process32FirstW(handle, ctypes.byref(entry)):
            raise Stop('WINDOWS_PROCESS_SNAPSHOT_UNAVAILABLE')
        pids = set()
        while True:
            pids.add(entry.pid)
            if not api.Process32NextW(handle, ctypes.byref(entry)):
                if ctypes.get_last_error() != 18:  # ERROR_NO_MORE_FILES
                    raise Stop('WINDOWS_PROCESS_SNAPSHOT_INCOMPLETE')
                break
        return pids
    finally:
        api.CloseHandle(handle)


def windows_listener_pids(port):
    if os.name != 'nt':
        raise Stop('WINDOWS_OWNER_LISTENER_INSPECTION_REQUIRED')
    from ctypes import wintypes as w
    api = ctypes.WinDLL('iphlpapi').GetExtendedTcpTable
    api.argtypes = [ctypes.c_void_p, ctypes.POINTER(w.DWORD), w.BOOL, w.ULONG, w.ULONG, w.ULONG]
    api.restype = w.DWORD
    size = w.DWORD(0)
    code = api(None, ctypes.byref(size), False, 2, 3, 0)  # AF_INET, OWNER_PID_LISTENER
    if code not in (0, 122):
        raise Stop('WINDOWS_TCP_TABLE_UNAVAILABLE')
    # Listeners may change between size and data calls. Retry boundedly, never
    # substitute a socket timeout or access error for evidence of absence.
    for _ in range(3):
        if not 4 <= size.value <= 4 * 1024 ** 2:
            raise Stop('WINDOWS_TCP_TABLE_SIZE_INVALID')
        data = ctypes.create_string_buffer(size.value)
        code = api(data, ctypes.byref(size), False, 2, 3, 0)
        if code == 0:
            return listener_pids_from_table(data.raw, port)
        if code != 122:
            raise Stop('WINDOWS_TCP_TABLE_UNAVAILABLE')
    raise Stop('WINDOWS_TCP_TABLE_UNSTABLE')


def cluster_process_and_listener_state(pid, port):
    if not 1 <= pid <= 0xFFFFFFFF:
        raise Stop('PRIOR_LOCAL_PROCESS_ID_INVALID')
    before = windows_process_ids()
    listeners = windows_listener_pids(port)
    after = windows_process_ids()
    return {'recorded_process_exists': pid in before or pid in after,
            'listener_pids': listeners,
            'probe': 'WINDOWS_PROCESS_SNAPSHOT_AND_IPV4_LISTENER_TABLE'}
