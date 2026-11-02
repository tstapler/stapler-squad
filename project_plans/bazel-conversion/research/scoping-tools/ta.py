from load import *
import fa,pickle,os
EDGES=json.load(open(S+'/edges.json'))
# normalize: edges by (UP,UF,DP,DF,Sym,Exp,Kind)
IDX=collections.defaultdict(list)
for e in EDGES:
    IDX[e['UP']].append(e)
def rp(p): return rel(p) if p.startswith(MOD) else p
byfile=collections.defaultdict(list)
for e in EDGES: byfile[(rp(e['UP']),e['UF'])].append(e)
def analyze(d):
    A=fa.analyze(d)
    nt={f['name'] for f in A['nt']}
    tt={f['name']:f for f in A['tt']}
    locs={f['name']:f['loc'] for f in A['nt']}
    uf=A['uf']
    # intra edges among non-test files
    fe=collections.defaultdict(lambda:collections.defaultdict(set)) # fe[a][b]={(sym,exp,kind)}
    tedges=collections.defaultdict(lambda:collections.defaultdict(set))
    for e in EDGES:
        if rp(e['UP'])==d and rp(e['DP'])==d:
            a=os.path.basename(e['UF']);b=os.path.basename(e['DF'])
            if os.path.dirname(e['UF'])!=d and not (d=='.' and os.path.dirname(e['UF'])==''): continue
            if os.path.dirname(e['DF'])!=d and not (d=='.' and os.path.dirname(e['DF'])==''): continue
            if a==b: continue
            if b not in nt: continue
            if a in nt: fe[a][b].add((e['Sym'],e['Exp'],e['Kind']))
            elif a in tt: tedges[a][b].add((e['Sym'],e['Exp'],e['Kind']))
    A.update(fe=fe,tedges=tedges,locs=locs,nt=nt,ttf=tt)
    return A
def gg(A):
    uf=A['uf'];g=collections.defaultdict(set);members=collections.defaultdict(list)
    for n in A['nt']: members[uf.f(n)].append(n)
    for a,bs in A['fe'].items():
        for b in bs:
            x,y=uf.f(a),uf.f(b)
            if x!=y: g[x].add(y)
    return g,members
if __name__=='__main__':
    import sys
    for d in sys.argv[1:]:
        A=analyze(d);g,m=gg(A)
        comps=fa.sccs(list(m),g);comps.sort(key=lambda c:-sum(len(m[x]) for x in c))
        print(d,'files',len(A['nt']),'groups',len(m),'SCCs',len(comps),'giant:',[(sum(len(m[x]) for x in c),sum(A['locs'][f] for x in c for f in m[x])) for c in comps[:3]])
