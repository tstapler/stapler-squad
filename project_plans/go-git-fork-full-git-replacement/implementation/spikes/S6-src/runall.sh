#!/bin/sh
cd /tmp/s6
for m in stock fresh fresh-retry reindex-retry reindex-retry-locked; do
  echo "\$ ./s6 -mode $m -maint repack -dur 60s"; ./s6 -mode $m -maint repack -dur 60s 2>&1 | head -30
done
for mt in gc packrefs; do for m in stock fresh-retry; do
  echo "\$ ./s6 -mode $m -maint $mt -dur 20s"; ./s6 -mode $m -maint $mt -dur 20s 2>&1 | head -30
done; done
echo DONE
