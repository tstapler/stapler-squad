from ta import *
import sys
for d in sys.argv[1:]:
    A=fa.analyze(d)
    r=sorted(((len(v),k) for k,v in A['meth_files'].items()),reverse=True)[:5]
    print(d,'top receivers by #files:',r)
    # files with methods on top receiver, LOC
    k=r[0][1];fs=A['meth_files'][k]
    locs={f['name']:f['loc'] for f in A['nt']}
    print('   ',k,'methods span',len(fs),'files',sum(locs[f] for f in fs),'LOC')
