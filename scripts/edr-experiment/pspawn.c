#include <spawn.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>
extern char **environ;
int main(int c,char**v){int dur=atoi(v[1]),rate=atoi(v[2]);struct timespec t0,t1;clock_gettime(CLOCK_MONOTONIC,&t0);int n=0;
char*argv[]={"/usr/bin/true",0};
while(1){clock_gettime(CLOCK_MONOTONIC,&t1);double el=(t1.tv_sec-t0.tv_sec)+(t1.tv_nsec-t0.tv_nsec)/1e9;if(el>dur)break;
 if(rate>0&&n>el*rate){usleep(1000);continue;}
 pid_t p;posix_spawn(&p,argv[0],0,0,argv,environ);int s;waitpid(p,&s,0);n++;}
printf("PSPAWN ops=%d rate=%.1f/s\n",n,n/(double)dur);}
