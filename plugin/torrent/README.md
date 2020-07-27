# torrent

## Name

*torrent* - use BitTorrent to disseminate zone data.

## Description

The *torrent* plugin uses the BitTorrent protocol to disseminate zone data. Multiple peers can
connect and down- and upload the data. A couple of nodes can be `seed` only meaning they will update
the torrent when their zone data changes. Non-`seed` peers will write received data back into the
zonefile - once the torrent is fully downloaded.

## Syntax

The simplest syntax is for a peer wanting to receive the zone data:

~~~ txt
torrent DBFILE
~~~

*  **DBFILE** the zone database file to torrent. If the path is relative, the path from the
   *root* plugin will be prepended to it.

For peers seeding the torrent use this, slightly expanded, syntax

~~~ txt
torrent DBFILE {
    seed
}
~~~

* `seed` tells *torrent* to seed content from **DBFILE** to the peers, it will _never_ write to
  **DBFILE**. When `seed` is _not_ specified **DBFILE** will be written to once the entire torrent
  is downloaded.

## Examples

~~~ txt
example.org {
    file db.example.org
    torrent db.example.org
}
~~~

## Also See

## Bugs
