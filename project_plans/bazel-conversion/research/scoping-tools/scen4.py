from scen2 import *
from peel import peel
def peel_map(d,B,prefix):
    R,peeled,C=peel(d,B,verbose=False)
    m=R['m'];A=R['A']
    pg=[g for g,*_ in peeled]
    # WCC over peeled groups via fe edges
    und=collections.defaultdict(set)
    uf=A['uf'];ps=set(pg)
    for a,bs in A['fe'].items():
        ga=uf.f(a)
        if ga not in ps: continue
        for b in bs:
            gb=uf.f(b)
            if gb in ps and gb!=ga: und[ga].add(gb);und[gb].add(ga)
    seen=set();comps=[]
    for g in pg:
        if g in seen: continue
        st=[g];c=set([g])
        while st:
            x=st.pop()
            for y in und[x]:
                if y not in c: c.add(y);st.append(y)
        seen|=c;comps.append(c)
    comps.sort(key=lambda c:-sum(len(m[x]) for x in c))
    f2n={};info=[]
    for i,c in enumerate(comps):
        nm=f"{d}/{prefix}{i}"
        fl=[f for x in c for f in m[x]]
        for f in fl: f2n[(d,f)]=nm
        info.append((nm,fl,sum(A['locs'][f] for f in fl)))
    return f2n,info,peeled,R
if __name__=='__main__':
    import sys
    B=int(sys.argv[1])
    f1,i1,p1,R1=peel_map('session',B,'p');f2,i2,p2,R2=peel_map('server/services',B,'p')
    print('session peel comps',[(n,len(fl),loc) for n,fl,loc in i1][:12],len(i1))
    print('services peel comps',[(n,len(fl),loc) for n,fl,loc in i2][:12],len(i2))
    inv=test_inversion_overrides()
    base=run('S0',{})[3]
    for label,f in (('peel session',f1),('peel services',f2),('S1+peel both',{**inv,**f1,**f2})):
        g,node,sg=build(f)[:3]
        # cut core->peeled edges
        for d in('session','server/services'):
            for n in list(g.lib):
                if n.startswith(d+'/p') and n[len(d)+2:].isdigit(): g.lib[d].discard(n)
        res=eval_graph(g,node,label);pc=per_commit(base,res,label)
        m5=pc['main500'];m1=pc['main100'];p3=pc['prs30']
        print(f"{label:16s} nodes={len(g.lib)} | main500 meanLOC {m5['mean_loc0']*100:.1f}->{m5['mean_loc1']*100:.1f} meanTestLOC {m5['mean_tloc0']*100:.1f}->{m5['mean_tloc1']*100:.1f} fewerLOC {m5['fewer_loc']*100:.1f}% fewerTest {m5['fewer_tloc']*100:.1f}% | main100 meanLOC {m1['mean_loc0']*100:.1f}->{m1['mean_loc1']*100:.1f} fewerLOC {m1['fewer_loc']*100:.1f}% | prs30 {p3['mean_loc0']*100:.1f}->{p3['mean_loc1']*100:.1f}")
