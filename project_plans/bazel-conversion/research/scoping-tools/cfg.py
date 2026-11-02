from ta import *
import sys
d=sys.argv[1]
use=collections.defaultdict(lambda:collections.defaultdict(set))
for e in EDGES:
    if rp(e['DP'])==d and rp(e['UP'])!=d and not e['UF'].endswith('_test.go'):
        use[os.path.basename(e['DF'])][rp(e['UP'])].add(e['Sym'])
A=analyze(d)
locs=A['locs']
for f in sorted(locs,key=lambda f:-len(use[f])):
    print(f"{f:40s} loc={locs[f]:5d} importers={len(use[f]):3d} syms={sum(len(v) for v in use[f].values())}")
