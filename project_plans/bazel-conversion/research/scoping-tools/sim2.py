from sim import *
# per-file typed deps
FDEPS=collections.defaultdict(set)   # (dir,name) -> set of (dp_dir, df_name)
for e in EDGES:
    up=rp(e['UP']);dp=rp(e['DP'])
    if isgen(dp) or isgen(up): continue
    ud=os.path.dirname(e['UF']) or '.';un=os.path.basename(e['UF'])
    dd=os.path.dirname(e['DF']) or '.';dn=os.path.basename(e['DF'])
    if (ud,un)==(dd,dn): continue
    if dd!=ud:
        fr=filemap.get((ud,un))
        if not fr or (MOD+'/'+dd if dd!='.' else MOD) not in {i.split(' ',1)[1] for i in (fr['imports'] or [])}: continue
    FDEPS[(ud,un)].add((dd,dn))
ALLPK={rel(p['ImportPath']) for p in PK if not isgen(rel(p['ImportPath']))}
def build(f2n_nt,test_assign=None):
    """f2n_nt: {(dir,name):node} overrides for non-test files; others default node=dir. returns G, straddle info"""
    node={}
    for f in FILES:
        k=(f['dir'],f['name'])
        if isgen(f['dir']) or f['dir'] not in ALLPK: continue
        if not f['test']:
            node[k]=f2n_nt.get(k,f['dir'])
    # tests
    straddle=[]
    for f in FILES:
        k=(f['dir'],f['name'])
        if not f['test'] or f['dir'] not in ALLPK: continue
        d=f['dir']
        if k in f2n_nt: node[k]=f2n_nt[k];continue
        split_nodes={node[(dd,nn)] for (dd,nn) in node if dd==d}
        if len(split_nodes)<=1: node[k]=d;continue
        base=f['name'][:-len('_test.go')]+'.go'
        if (d,base) in node: node[k]=node[(d,base)];continue
        c=collections.Counter(node[x] for x in FDEPS.get(k,()) if x[0]==d and x in node)
        if c:
            node[k]=c.most_common(1)[0][0]
            if len(c)>1: straddle.append((k,dict(c)))
        else: node[k]=d
    g=G()
    nodes={node[k] for k in node}
    for n in nodes: g.lib[n]=set();g.test[n]=set()
    for k,n in node.items():
        isT=k[1].endswith('_test.go')
        tgt=g.test if isT else g.lib
        for x in FDEPS.get(k,()):
            if x in node and node[x]!=n: tgt[n].add(node[x])
        if isT: g.hastest.add(n)
    # blank/unused imports: ensure pkg-level import edges from go list are present at least at dir level for unsplit
    splitdirs={k[0] for k,v in f2n_nt.items()}
    for p in PK:
        d=rel(p['ImportPath'])
        if d not in ALLPK or d in splitdirs: continue
        for i in (p.get('Imports') or []):
            if inmod(i) and rel(i) in ALLPK and rel(i) not in splitdirs and rel(i)!=d: g.lib[d].add(rel(i))
        for i in (p.get('TestImports') or [])+(p.get('XTestImports') or []):
            if inmod(i) and rel(i) in ALLPK and rel(i) not in splitdirs and rel(i)!=d: g.test[d].add(rel(i))
    for k,n in node.items():
        if not k[1].endswith('_test.go'):
            g.loc[n]=g.loc.get(n,0)+filemap[k]['loc']
    for n in g.lib: g.loc.setdefault(n,0)
    return g,node,straddle
def compare_base():
    g0=base_graph();g1,node,_=build({})
    diffs=0
    for p in g0.lib:
        a=g0.lib[p];b=g1.lib.get(p,set())
        if a!=b: diffs+=1; 
        if a!=b and diffs<6: print('lib diff',p,'only go list:',a-b,'only typed:',b-a)
    for p in g0.test:
        a=g0.test[p]-{p};b=g1.test.get(p,set())-{p}
        if a!=b: diffs+=1
        if a!=b and diffs<10: print('test diff',p,a^b)
    print('identity rebuild diffs',diffs,'of',len(g0.lib))
    return g0,g1
if __name__=='__main__': compare_base()
