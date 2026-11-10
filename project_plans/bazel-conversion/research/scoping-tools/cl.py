from ta import *
def classify(d,verbose=True,minfiles=3):
    A=analyze(d);g,m=gg(A);uf=A['uf']
    comps=fa.sccs(list(m),g);comps.sort(key=lambda c:-sum(len(m[x]) for x in c))
    giant=set(comps[0])
    rg=collections.defaultdict(set)
    for a,bs in g.items():
        for b in bs: rg[b].add(a)
    def reach(start,adj):
        seen=set(start);st=list(start)
        while st:
            x=st.pop()
            for y in adj.get(x,()):
                if y not in seen: seen.add(y);st.append(y)
        return seen
    below=reach(giant,g)-giant;above=reach(giant,rg)-giant
    indep=set(m)-giant-below-above
    cls={}
    for x in giant: cls[x]='core'
    for x in below: cls[x]='below'
    for x in above: cls[x]='above'
    for x in indep: cls[x]='indep'
    files=lambda S_:[f for x in S_ for f in m[x]]
    def stat(S_):
        fl=files(S_);return len(fl),sum(A['locs'][f] for f in fl)
    res=dict(A=A,m=m,g=g,cls=cls)
    if verbose:
        print('\n==',d,'files',len(A['nt']),'LOC',sum(A['locs'].values()))
        for c,S_ in (('core',giant),('below',below),('above',above),('indep',indep)):
            print(' ',c,'files/LOC',stat(S_))
    # weakly connected comps of non-core groups
    nc=[x for x in m if cls[x]!='core']
    und=collections.defaultdict(set)
    for a in nc:
        for b in g.get(a,()):
            if b in cls and cls[b]!='core' : und[a].add(b);und[b].add(a)
    seen=set();wcc=[]
    for x in nc:
        if x in seen: continue
        comp=reach([x],und);seen|=comp;wcc.append(comp)
    wcc.sort(key=lambda c:-sum(len(m[x]) for x in c))
    res['wcc']=wcc
    return res
def boundary(R,S_files,direction):
    """symbols crossing the boundary of file set S_files; direction 'in': rest->S uses S's symbols; 'out': S->rest"""
    A=R['A'];fe=A['fe']
    S_=set(S_files);inn=collections.defaultdict(set);out=collections.defaultdict(set)
    userfiles_in=set();userfiles_out=set()
    for a,bs in fe.items():
        for b,syms in bs.items():
            if a in S_ and b not in S_:
                for s in syms: out[s].add(a)
            if b in S_ and a not in S_:
                for s in syms: inn[s].add(a)
    return inn,out
def describe(R,comp,name=None):
    m=R['m'];A=R['A']
    fl=[f for x in comp for f in m[x]]
    inn,out=boundary(R,fl,None)
    inun={s for s in inn if not s[1]};outun={s for s in out if not s[1]}
    return dict(files=len(fl),loc=sum(A['locs'][f] for f in fl),
        cls=collections.Counter(R['cls'][x] for x in comp).most_common(),
        in_syms=len(inn),in_unexp=len(inun),out_syms=len(out),out_unexp=len(outun),
        names=sorted(fl)[:6])
if __name__=='__main__':
    import sys
    for d in sys.argv[1:]:
        R=classify(d)
        for c in R['wcc'][:12]:
            ds=describe(R,c)
            print('  WCC',ds)
