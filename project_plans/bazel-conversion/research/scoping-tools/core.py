from cl import *
import sys
d=sys.argv[1]
R=classify(d,verbose=False);A=R['A'];m=R['m'];cls=R['cls'];uf=A['uf']
core=[x for x in m if cls[x]=='core']
locs=A['locs']
# label group by largest file name; list by size
def gl(x): return sum(locs[f] for f in m[x])
core.sort(key=lambda x:-gl(x))
print('core groups',len(core),'top by LOC:')
for x in core[:14]:
    print(' ',len(m[x]),'files',gl(x),'LOC',sorted(m[x])[:4])
# mutual edge counts between top groups
top=core[:8]
def name(x): return sorted(m[x])[0]
print('edge syms between top groups (a->b distinct syms):')
for a in top:
    row=[]
    for b in top:
        if a==b: row.append('  .');continue
        s=set()
        for fa_ in m[a]:
            for fb,syms in A['fe'].get(fa_,{}).items():
                if uf.f(fb)==b: s|=syms
        row.append(f"{len(s):3d}")
    print(f"{name(a):34s}"," ".join(row))
