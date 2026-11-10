from cl import *
import sys
d=sys.argv[1];top=int(sys.argv[2]) if len(sys.argv)>2 else 14
R=classify(d,verbose=False);A=R['A'];m=R['m'];cls=R['cls'];uf=A['uf'];fe=A['fe']
core=[x for x in m if cls[x]=='core']
locs=A['locs']
gl=lambda x:sum(locs[f] for f in m[x])
core.sort(key=lambda x:-gl(x))
allcore_files={f for x in core for f in m[x]}
print('group (files,LOC) | inbound syms from rest-of-core (the back-edges to cut) | outbound syms to rest-of-core | inbound unexported | inbound symbol list')
for x in core[:top]:
    G=set(m[x]);inn=set();out=set()
    for a,bs in fe.items():
        for b,syms in bs.items():
            if a in allcore_files and b in allcore_files:
                if a not in G and b in G: inn|={(b,)+s for s in syms}
                if a in G and b not in G: out|={(a,)+s for s in syms}
    print(f"{sorted(G)[0]:36s} ({len(G)},{gl(x)}) in={len({s[1] for s in inn})} out={len({s[1] for s in out})} in_unexp={len({s[1] for s in inn if not s[2]})} {sorted({s[1] for s in inn})[:8]}")
