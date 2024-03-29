### Ping many IP addresses rapidly.

IP is
- IP address, e.g. 1.2.3.4
- IP prefix, e.g. 1.2.3.0/24
- IP range, e.g. 1.2.3.4-1.2.3.42

Both v4 and v6 addresses are accepted.

Uses unprivileged ICMP connection, so ``/proc/sys/net/ipv4/ping_group_range`` must be set accordingly.

### ``multi-ping filename``
Reads IP addresses from filename.

### ``multi-ping``
Reads IP addresses from STDIN.

### Options

``-d duration`` Delay between successive pings. Duration is given as number followed by timeunit (s, ms, ns).

``-t duration`` Timeout for echo reply.

``-r`` Show reachable IP addresses line by line.

``-u`` Show unreachable IP addresses line by line.

If both or non of ``-r`` and ``-u`` is given, all addresses are shown and marked "ok" or "failed" respectively.

``-D`` Print debug messages.
