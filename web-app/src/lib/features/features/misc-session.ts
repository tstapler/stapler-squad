import type { Feature } from '../types';

export const miscSessionFeatures = {
  'hibernate-session': {
    id: 'hibernate-session',
    title: 'Hibernate Session',
    description: 'Hibernates a running session, persisting its state and suspending its process.',
    rpcIds: ['HibernateSession'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
  'resume-hibernated-session': {
    id: 'resume-hibernated-session',
    title: 'Resume Hibernated Session',
    description: 'Resumes a previously hibernated session, restoring its process from saved state.',
    rpcIds: ['ResumeHibernatedSession'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
  'list-session-tokens': {
    id: 'list-session-tokens',
    title: 'List Session Tokens',
    description: 'Lists token usage records associated with a session for cost tracking.',
    rpcIds: ['ListSessionTokens'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
  'write-to-session': {
    id: 'write-to-session',
    title: 'Write To Session',
    description: 'Writes input text directly to a session terminal as if typed by the user.',
    rpcIds: ['WriteToSession'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
