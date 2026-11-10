from cl import *
import sys
def prefix(files):
    c=collections.Counter(f.split('_')[0].replace('.go','') for f in files)
    return c.most_common(3)
def propose(d,minfiles=3,classes=('above','indep')):
    R=classify(d,verbose=False);m=R['m'];A=R['A'];cls=R['cls']
    sats=[]
    for comp in R['wcc']:
        fl=[f for x in comp if cls[x] in classes for f in m[x]]
        if len(fl)<minfiles: continue
        sats.append(fl)
    return R,sats
if __name__=='__main__':
    d=sys.argv[1];mf=int(sys.argv[2]) if len(sys.argv)>2 else 3
    R,sats=propose(d,mf)
    A=R['A']
    tot=0
    for fl in sats:
        inn,out=boundary(R,fl,None)
        outun={s for s in out if not s[1]};inun={s for s in inn if not s[1]}
        loc=sum(A['locs'][f] for f in fl);tot+=loc
        # tests: files whose name base matches
        print(f"files={len(fl):3d} loc={loc:5d} prefixes={prefix(fl)} out_syms(core->needed by sat)={len(out)} unexported_to_export={len(outun)} in(core uses sat)={len(inn)} {sorted(fl)[:3]}")
    print('total satellite LOC',tot,'of',sum(A['locs'].values()))
