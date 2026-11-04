from scen4 import *
def utouch(node,U,commits):
    tot=0;hit=0
    for c in commits:
        ns=set()
        for path in c:
            if path.endswith('.go') and not path.endswith('_test.go'):
                d=os.path.dirname(path) or '.';ns.add(node.get((d,os.path.basename(path)),d))
        if not ns: continue
        tot+=1
        if ns&U: hit+=1
    return hit,tot
cs=[]
cur=None
for l in open(S+'/gitlog500.txt'):
    l=l.rstrip('\n')
    if l.startswith('@@'): cur=[];cs.append(cur)
    elif l and cur is not None: cur.append(l)
U={'session','server/services','server','config','.'}
sess,_=mk_sats('session',SESS_NAMES);svc,_=mk_sats('server/services',SVC_NAMES)
f1,i1,_,_=peel_map('session',10,'p');f2,i2,_,_=peel_map('server/services',10,'p')
for label,f in (('baseline',{}),('S2 session mech sats',sess),('S3 services mech sats',svc),('S2+S3',{**sess,**svc}),('peel B=10 both',{**f1,**f2}),('S2+S3+peel',{**sess,**svc,**f1,**f2})):
    g,node,_=build(f)
    for nm,sl in (('main100',cs[:100]),('main500',cs)):
        h,t=utouch(node,U,sl)
        print(f"{label:24s} {nm}: commits touching a universal-slice package (core session/services/server/config/main): {h}/{t} = {100*h/t:.0f}%")
