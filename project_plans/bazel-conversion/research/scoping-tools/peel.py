from cl import *
import sys
def peel(d,B,minloc=0,verbose=True):
    R=classify(d,verbose=False);A=R['A'];m=R['m'];cls=R['cls'];uf=A['uf'];fe=A['fe'];locs=A['locs']
    core=[x for x in m if cls[x]=='core']
    gl=lambda x:sum(locs[f] for f in m[x])
    # group-level sym edges
    E=collections.defaultdict(lambda:collections.defaultdict(set))  # E[a][b] syms of b used by a
    for a,bs in fe.items():
        ga=uf.f(a)
        if ga not in core: continue
        for b,syms in bs.items():
            gb=uf.f(b)
            if gb in core and gb!=ga: E[ga][gb]|={(b,)+s for s in syms}
    C=set(core);giant=max(core,key=gl);peeled=[]
    while True:
        best=None
        for g in C:
            if g==giant: continue
            inn=set()
            for h in C:
                if h!=g: inn|={s[1:] for s in E[h].get(g,set())}
            if len(inn)<=B and gl(g)>=minloc:
                key=(len(inn),-gl(g))
                if best is None or key<best[0]: best=(key,g,inn)
        if not best: break
        _,g,inn=best;C.discard(g)
        outs=set();
        for h in C: outs|={s[1:] for s in E[g].get(h,set())}
        peeled.append((g,len(inn),sorted(x[0] for x in inn)[:6],len(outs),len({o for o in outs if not o[1]})))
    if verbose:
        print(d,'budget',B,': peeled groups',len(peeled),'files',sum(len(m[g]) for g,*_ in peeled),'LOC',sum(gl(g) for g,*_ in peeled),'| core remains files',sum(len(m[x]) for x in C),'LOC',sum(gl(x) for x in C))
        for g,k,names,o,ou in peeled: print(f"   {sorted(m[g])[0]:42s} files={len(m[g]):2d} loc={gl(g):5d} inbound_syms_to_move={k} {names} uses_core_syms={o} (unexported {ou})")
    return R,peeled,C
if __name__=='__main__':
    for d in sys.argv[2:]: peel(d,int(sys.argv[1]),minloc=0)
