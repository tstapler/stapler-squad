from scen2 import *
def ra1_merged(g,node,paths_list):
    gi=set()
    for p in PK:
        d=rel(p['ImportPath'])
        if isgen(d): continue
        for i in (p.get('Imports') or []):
            if inmod(i) and rel(i).startswith('gen/'): gi.add(d)
    gi_nodes={n for k,n in node.items() if k[0] in gi and not k[1].endswith('_test.go')}|(gi&set(g.lib))
    fr=[]
    for paths in paths_list:
        ch=set()
        for path in paths:
            if path.endswith('.go'):
                d=os.path.dirname(path) or '.';nm=os.path.basename(path);n=node.get((d,nm),d)
                if n in g.lib: ch.add(n)
            elif path.startswith('proto/'): ch|=gi_nodes
            elif path.startswith('web-app/') and 'server/web' in g.lib: ch.add('server/web')
        fr.append(merged_frac(g,ch) if ch else 0)
    return fr
prs=[p['paths'] for p in json.load(open(B))['partial']]
cs=[]
cur=None
for l in open(S+'/gitlog500.txt'):
    l=l.rstrip('\n')
    if l.startswith('@@'): cur=[];cs.append(cur)
    elif l and cur is not None: cur.append(l)
if __name__=='__main__':
    sess,sinfo=mk_sats('session',SESS_NAMES);svc,vinfo=mk_sats('server/services',SVC_NAMES);inv=test_inversion_overrides()
    A=ta.analyze('session')
    bl=set(session_files(lambda n: n.startswith('backlog') or '_backlog' in n))
    for t,fs in A['meth_files'].items():
        if t=='BacklogLifecycleListener': bl|=set(fs)
    sc={'S0':({}),'S1 test back-edges only':dict(inv),'S2 session sats only':dict(sess),'S3 services sats only':dict(svc),'S4 S1+S2+S3':{**sess,**svc,**inv},
        'P2a S4+backlog seam':({**sess,**svc,**inv,**{('session',f):'session/backlog' for f in bl}})}
    for k,f in sc.items():
        g,node,sg=build(f)
        if 'backlog' in k: g.lib['session'].discard('session/backlog')
        fr=ra1_merged(g,node,prs); gt=[x for x in fr if x>0]
        fr2=ra1_merged(g,node,cs[:100]); gt2=[x for x in fr2 if x>0]
        print(f"{k:28s} N={len(g.lib)} RA1-merged median(all30)={statistics.median(fr):.3f} goTouching={statistics.median(gt):.3f} max={max(fr):.3f} | main100 goTouching median={statistics.median(gt2):.3f} mean={sum(gt2)/len(gt2):.3f}")
        # sep model count fraction
        res=eval_graph(g,node,k);sm=summarize(res,k)
        print('    sep-model target frac prs30 GoTouching median=%.3f  main100 median=%.3f mean-target-frac main100=%.3f'%(sm['prs30']['med_frac'],sm['main100']['med_frac'],sum(r['tgt'] for r in res['out']['main100'])/len(res['out']['main100'])/(res['NL']+res['NT'])))
