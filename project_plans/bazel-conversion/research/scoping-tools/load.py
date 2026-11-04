import json,sys,os,subprocess,collections,re
R=os.environ.get('REPO_ROOT','.')
MOD='github.com/tstapler/stapler-squad'
def load():
    raw=open(os.path.join(os.environ.get('SCOPE_DIR','.'),'golist.json')).read()
    dec=json.JSONDecoder();i=0;pk=[]
    while i<len(raw):
        while i<len(raw) and raw[i].isspace(): i+=1
        if i>=len(raw): break
        o,j=dec.raw_decode(raw,i);pk.append(o);i=j
    return pk
def rel(p): return p[len(MOD)+1:] if p!=MOD else '.'
S=os.environ.get('SCOPE_DIR','.')
def isgen(d): return d.startswith('gen/') or (d.startswith('session/ent') and d!='session/ent/schema')
def graph(pk,include_tests=True,skipgen=True):
    edges={}
    for p in pk:
        d=rel(p['ImportPath'])
        if skipgen and isgen(d): continue
        im=list(p.get('Imports') or [])
        if include_tests: im+=list(p.get('TestImports') or [])+list(p.get('XTestImports') or [])
        edges[d]={rel(i) for i in im if i.startswith(MOD+'/') or i==MOD}
    return edges
def commits():
    cs=[];cur=None
    for l in open(S+'/gitlog500.txt'):
        l=l.rstrip('\n')
        if l.startswith('@@'): cur=set();cs.append(cur)
        elif l and cur is not None: cur.add(l)
    return cs
def rdeps_closure(edges):
    rev=collections.defaultdict(set)
    for p,im in edges.items():
        for i in im: rev[i].add(p)
    memo={}
    def tr(p):
        seen=set();st=[p]
        while st:
            x=st.pop()
            for y in rev.get(x,()):
                if y not in seen: seen.add(y);st.append(y)
        return seen
    return rev,{p:tr(p) for p in edges}
