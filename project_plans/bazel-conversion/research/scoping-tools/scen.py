from sim2 import *
from prop import propose
import statistics
SLUG={}
def mk_sats(d,names,minfiles=3):
    R,sats=propose(d,minfiles)
    f2n={};info=[]
    for i,fl in enumerate(sats):
        nm=names[i] if i<len(names) else None
        if nm is None: continue
        for f in fl: f2n[(d,f)]=d+'/'+nm
        info.append((d+'/'+nm,fl))
    return f2n,info
SESS_NAMES=['backlogplugin','importer','handoff','paneprompt','pipelinemode','sweeper','prpoller','external']
SVC_NAMES=['hookctx','insights','wsbridge','gatestats','webhooks','sessionbundle','devtools']
def test_inversion_overrides():
    """test files whose imports create layer inversions -> own '<pkg>/inttest' node"""
    g=base_graph()
    def clos(p):
        seen=set();st=[p]
        while st:
            x=st.pop()
            for y in g.lib.get(x,()):
                if y not in seen: seen.add(y);st.append(y)
        return seen
    out={}
    for p,t in g.test.items():
        bad={q for q in t if q!=p and p in clos(q)}
        if not bad: continue
        for f in FILES:
            if f['dir']==p and f['test']:
                imps={i.split(' ',1)[1][len(MOD)+1:] for i in (f['imports'] or []) if i.split(' ',1)[1].startswith(MOD+'/')}
                if imps&bad: out[(p,f['name'])]=p+'/inttest'
    return out
def eval_graph(g,node,label,verbose=True):
    # changed-sets
    genimp={n for n in g.lib if any(k for k in [])}
    prs=json.load(open(B))['partial']
    # gen importers (unsplit packages only, from original go list)
    gi=set()
    for p in PK:
        d=rel(p['ImportPath'])
        if isgen(d): continue
        for i in (p.get('Imports') or []):
            if inmod(i) and rel(i).startswith('gen/'): gi.add(d)
    gi_nodes={n for k,n in node.items() if k[0] in gi and not k[1].endswith('_test.go')} | (gi&set(g.lib))
    def nodes_of(paths):
        L=set();T=set()
        for path in paths:
            if path.endswith('.go'):
                d=os.path.dirname(path) or '.';nm=os.path.basename(path)
                n=node.get((d,nm),d)
                if n not in g.lib: continue
                (T if nm.endswith('_test.go') else L).add(n)
            elif path.startswith('proto/'): L|=gi_nodes
            elif path.startswith('web-app/') and 'server/web' in g.lib: L.add('server/web')
        return L,T
    # commits
    cs=[]
    cur=None
    for l in open(S+'/gitlog500.txt'):
        l=l.rstrip('\n')
        if l.startswith('@@'): cur=[];cs.append(cur)
        elif l and cur is not None: cur.append(l)
    sets={'prs30':[pr['paths'] for pr in prs],'main100':cs[:100],'main500':cs[:500]}
    tloc={}
    for k,n in node.items():
        if k[1].endswith('_test.go'): tloc[n]=tloc.get(n,0)+filemap[k]['loc']
    out={}
    for nm,lst in sets.items():
        rows=[]
        for paths in lst:
            L,T=nodes_of(paths)
            if not L and not T: continue
            libs,tests=affected(g,L,T)
            rows.append(dict(libs=len(libs),tests=len(tests),tgt=len(libs)+len(tests),
                 loc=sum(g.loc[p] for p in libs),tloc=sum(tloc.get(p,0) for p in tests)))
        out[nm]=rows
    NL=len(g.lib);NT=len(g.hastest);TL=sum(g.loc.values());TT=sum(tloc.values())
    return dict(out=out,NL=NL,NT=NT,TL=TL,TT=TT)
def summarize(res,label):
    NL,NT,TL,TT=res['NL'],res['NT'],res['TL'],res['TT']
    s={}
    for nm,rows in res['out'].items():
        if not rows: continue
        s[nm]=dict(n=len(rows),med_tgt=statistics.median(r['tgt'] for r in rows),med_frac=statistics.median(r['tgt'] for r in rows)/(NL+NT),
            med_libs=statistics.median(r['libs'] for r in rows),med_tests=statistics.median(r['tests'] for r in rows),
            med_loc=statistics.median(r['loc'] for r in rows),med_locfrac=statistics.median(r['loc'] for r in rows)/TL,
            mean_locfrac=sum(r['loc'] for r in rows)/len(rows)/TL,
            med_tlocfrac=statistics.median(r['tloc'] for r in rows)/TT,mean_tlocfrac=sum(r['tloc'] for r in rows)/len(rows)/TT)
    return s
def run(label,f2n):
    g,node,st=build(f2n)
    res=eval_graph(g,node,label)
    return g,node,st,res
if __name__=='__main__':
    sess,sinfo=mk_sats('session',SESS_NAMES);svc,vinfo=mk_sats('server/services',SVC_NAMES)
    inv=test_inversion_overrides()
    scen={'S0 baseline':{},'S1 test-backedge':dict(inv),'S2 session sats':dict(sess),'S3 services sats':dict(svc),'S5 all':{**sess,**svc,**inv}}
    allres={}
    for k,f in scen.items():
        g,node,st,res=run(k,f)
        # inversion tests: need node override for tests -> build() assigns tests by name/majority; apply overrides explicit
        allres[k]=(g,summarize(res,k),len(g.lib),len(g.hastest))
        print(k,'nodes',len(g.lib),'test nodes',len(g.hastest),'straddling tests',len(st))
        for nm,v in allres[k][1].items(): print('  ',nm,{a:(round(b,4) if isinstance(b,float) else b) for a,b in v.items()})

def per_commit(res0,res1,key):
    out={}
    for nm in res0['out']:
        a=res0['out'][nm];b=res1['out'][nm]
        assert len(a)==len(b),(nm,len(a),len(b))
        out[nm]=dict(
          mean_loc0=sum(r['loc'] for r in a)/len(a)/res0['TL'],mean_loc1=sum(r['loc'] for r in b)/len(b)/res1['TL'],
          mean_tloc0=sum(r['tloc'] for r in a)/len(a)/res0['TT'],mean_tloc1=sum(r['tloc'] for r in b)/len(b)/res1['TT'],
          med_loc0=statistics.median(r['loc'] for r in a)/res0['TL'],med_loc1=statistics.median(r['loc'] for r in b)/res1['TL'],
          fewer_loc=sum(1 for x,y in zip(a,b) if y['loc']/res1['TL']<x['loc']/res0['TL']-1e-9)/len(a),
          fewer_tloc=sum(1 for x,y in zip(a,b) if y['tloc']/res1['TT']<x['tloc']/res0['TT']-1e-9)/len(a),
          med_tgt0=statistics.median(r['tgt'] for r in a),med_tgt1=statistics.median(r['tgt'] for r in b),n=len(a))
    return out
