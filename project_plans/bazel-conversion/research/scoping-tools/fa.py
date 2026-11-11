# file-level analysis of one package
from load import *
FILES=json.load(open(S+'/files.json'))
def pkgfiles(d,tests=False):
    return [f for f in FILES if f['dir']==d and f['test']==tests and f['name'].endswith('.go')]
class UF:
    def __init__(s): s.p={}
    def f(s,x):
        s.p.setdefault(x,x)
        while s.p[x]!=x: s.p[x]=s.p[s.p[x]];x=s.p[x]
        return x
    def u(s,a,b): s.p[s.f(a)]=s.f(b)
def analyze(d):
    nt=pkgfiles(d);tt=pkgfiles(d,True)
    # tests in same package name (not _test pkg) are internal
    decl_file={};meth_files=collections.defaultdict(set);meth_name_types=collections.defaultdict(set);meth_decl={}
    for f in nt:
        for x in f['decls'] or []:
            if x['k']=='method':
                meth_files[x['r']].add(f['name']);meth_name_types[x['n']].add(x['r']);meth_decl.setdefault(x['n'],set()).add(f['name'])
            else:
                decl_file.setdefault(x['n'],f['name'])
    uf=UF()
    for f in nt: uf.f(f['name'])
    for t,fs in meth_files.items():
        fs=set(fs)
        if t in decl_file: fs.add(decl_file[t])
        fs=list(fs)
        for a in fs[1:]: uf.u(fs[0],a)
    names={f['name']:f for f in nt}
    edges=collections.defaultdict(lambda: collections.Counter())
    def refs(f):
        out=collections.Counter()
        for i in f['idents'] or []:
            if i in decl_file and decl_file[i]!=f['name']: out[(decl_file[i],i)]+=1
        for x,s in f['sels'] or []:
            # method selectors with unique method name
            if s in meth_decl and len(meth_name_types[s])==1 and x not in ('',):
                for mf in meth_decl[s]:
                    if mf!=f['name']: out[(mf,s)]+=1
        return out
    fr={f['name']:refs(f) for f in nt}
    return dict(nt=nt,tt=tt,uf=uf,decl_file=decl_file,fr=fr,meth_files=meth_files)
def sccs(nodes,adj):
    idx={};low={};st=[];on=set();res=[];c=[0]
    import sys;sys.setrecursionlimit(100000)
    def go(v):
        idx[v]=low[v]=c[0];c[0]+=1;st.append(v);on.add(v)
        for w in adj.get(v,()):
            if w not in idx: go(w);low[v]=min(low[v],low[w])
            elif w in on: low[v]=min(low[v],idx[w])
        if low[v]==idx[v]:
            comp=[]
            while True:
                w=st.pop();on.discard(w);comp.append(w)
                if w==v:break
            res.append(comp)
    for v in nodes:
        if v not in idx: go(v)
    return res
def groups_graph(A):
    uf=A['uf'];g=collections.defaultdict(set);members=collections.defaultdict(list)
    for f in A['nt']: members[uf.f(f['name'])].append(f['name'])
    for fn,r in A['fr'].items():
        for (tf,sym) in r:
            a,b=uf.f(fn),uf.f(tf)
            if a!=b: g[a].add(b)
    return g,members
if __name__=='__main__':
    import sys
    d=sys.argv[1];A=analyze(d)
    g,members=groups_graph(A)
    nodes=list(members)
    comps=sccs(nodes,g)
    comps.sort(key=lambda c:-sum(len(members[x]) for x in c))
    locs={f['name']:f['loc'] for f in A['nt']}
    print(d,'files',len(A['nt']),'groups(method-bound)',len(members),'SCCs',len(comps))
    for c in comps[:5]:
        fl=[m for x in c for m in members[x]]
        print(' SCC files',len(fl),'LOC',sum(locs[m] for m in fl))
