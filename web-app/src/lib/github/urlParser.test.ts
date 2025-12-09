/**
 * Tests for GitHub URL Parser - mirrors github/url_parser_test.go
 */

import {
  parseGitHubRef,
  isGitHubRef,
  getCloneUrl,
  getHtmlUrl,
  getDisplayName,
  getSuggestedSessionName,
  getRepoFullName,
  RefType,
  type ParsedGitHubRef,
} from './urlParser';

describe('parseGitHubRef', () => {
  interface TestCase {
    name: string;
    input: string;
    wantType?: RefType;
    wantOwner?: string;
    wantRepo?: string;
    wantPR?: number;
    wantBranch?: string;
    wantNull?: boolean;
  }

  const testCases: TestCase[] = [
    // PR URLs
    {
      name: 'PR URL with https',
      input: 'https://github.com/owner/repo/pull/123',
      wantType: RefType.PR,
      wantOwner: 'owner',
      wantRepo: 'repo',
      wantPR: 123,
    },
    {
      name: 'PR URL without https',
      input: 'github.com/owner/repo/pull/456',
      wantType: RefType.PR,
      wantOwner: 'owner',
      wantRepo: 'repo',
      wantPR: 456,
    },
    {
      name: 'PR URL with trailing path',
      input: 'https://github.com/owner/repo/pull/789/files',
      wantType: RefType.PR,
      wantOwner: 'owner',
      wantRepo: 'repo',
      wantPR: 789,
    },

    // Branch URLs
    {
      name: 'Branch URL with https',
      input: 'https://github.com/owner/repo/tree/feature-branch',
      wantType: RefType.Branch,
      wantOwner: 'owner',
      wantRepo: 'repo',
      wantBranch: 'feature-branch',
    },
    {
      name: 'Branch URL with slashes in branch',
      input: 'https://github.com/owner/repo/tree/feature/my-feature',
      wantType: RefType.Branch,
      wantOwner: 'owner',
      wantRepo: 'repo',
      wantBranch: 'feature/my-feature',
    },
    {
      name: 'Branch URL without https',
      input: 'github.com/owner/repo/tree/main',
      wantType: RefType.Branch,
      wantOwner: 'owner',
      wantRepo: 'repo',
      wantBranch: 'main',
    },

    // Repository URLs
    {
      name: 'Repo URL with https',
      input: 'https://github.com/owner/repo',
      wantType: RefType.Repo,
      wantOwner: 'owner',
      wantRepo: 'repo',
    },
    {
      name: 'Repo URL with https and trailing slash',
      input: 'https://github.com/owner/repo/',
      wantType: RefType.Repo,
      wantOwner: 'owner',
      wantRepo: 'repo',
    },
    {
      name: 'Repo URL with .git suffix',
      input: 'https://github.com/owner/repo.git',
      wantType: RefType.Repo,
      wantOwner: 'owner',
      wantRepo: 'repo',
    },
    {
      name: 'Repo URL without https',
      input: 'github.com/owner/repo',
      wantType: RefType.Repo,
      wantOwner: 'owner',
      wantRepo: 'repo',
    },

    // Shorthand formats
    {
      name: 'Shorthand with branch',
      input: 'owner/repo:feature-branch',
      wantType: RefType.Branch,
      wantOwner: 'owner',
      wantRepo: 'repo',
      wantBranch: 'feature-branch',
    },
    {
      name: 'Shorthand repo only',
      input: 'owner/repo',
      wantType: RefType.Repo,
      wantOwner: 'owner',
      wantRepo: 'repo',
    },
    {
      name: 'Shorthand with hyphenated names',
      input: 'my-org/my-repo',
      wantType: RefType.Repo,
      wantOwner: 'my-org',
      wantRepo: 'my-repo',
    },
    {
      name: 'Shorthand with branch containing slashes',
      input: 'owner/repo:feature/my-feature',
      wantType: RefType.Branch,
      wantOwner: 'owner',
      wantRepo: 'repo',
      wantBranch: 'feature/my-feature',
    },

    // Real-world examples
    {
      name: 'claude-squad repo',
      input: 'https://github.com/anthropics/claude-squad',
      wantType: RefType.Repo,
      wantOwner: 'anthropics',
      wantRepo: 'claude-squad',
    },
    {
      name: 'claude-squad PR',
      input: 'https://github.com/anthropics/claude-squad/pull/42',
      wantType: RefType.PR,
      wantOwner: 'anthropics',
      wantRepo: 'claude-squad',
      wantPR: 42,
    },

    // Error cases
    {
      name: 'Empty input',
      input: '',
      wantNull: true,
    },
    {
      name: 'Random text',
      input: 'hello world',
      wantNull: true,
    },
    {
      name: 'Local path',
      input: '/Users/test/project',
      wantNull: true,
    },
    {
      name: 'Non-GitHub URL',
      input: 'https://gitlab.com/owner/repo',
      wantNull: true,
    },
  ];

  testCases.forEach((tt) => {
    it(tt.name, () => {
      const got = parseGitHubRef(tt.input);

      if (tt.wantNull) {
        expect(got).toBeNull();
        return;
      }

      expect(got).not.toBeNull();
      if (!got) return; // Type guard

      expect(got.type).toBe(tt.wantType);
      expect(got.owner).toBe(tt.wantOwner);
      expect(got.repo).toBe(tt.wantRepo);
      expect(got.originalUrl).toBe(tt.input.trim());

      if (tt.wantPR !== undefined) {
        expect(got.prNumber).toBe(tt.wantPR);
      }
      if (tt.wantBranch !== undefined) {
        expect(got.branch).toBe(tt.wantBranch);
      }
    });
  });
});

describe('isGitHubRef', () => {
  interface TestCase {
    input: string;
    want: boolean;
  }

  const testCases: TestCase[] = [
    // True cases
    { input: 'https://github.com/owner/repo', want: true },
    { input: 'github.com/owner/repo', want: true },
    { input: 'owner/repo', want: true },
    { input: 'owner/repo:branch', want: true },
    { input: 'https://github.com/owner/repo/pull/123', want: true },
    { input: 'https://github.com/owner/repo/tree/main', want: true },
    { input: 'my-org/my-repo', want: true },

    // False cases
    { input: '', want: false },
    { input: 'hello', want: false },
    { input: '/local/path', want: false },
    { input: 'https://gitlab.com/owner/repo', want: false },
    { input: 'owner', want: false },
    { input: '-invalid/repo', want: false },
  ];

  testCases.forEach((tt) => {
    it(`"${tt.input}" should return ${tt.want}`, () => {
      expect(isGitHubRef(tt.input)).toBe(tt.want);
    });
  });
});

describe('Helper functions', () => {
  describe('getCloneUrl', () => {
    it('returns correct clone URL', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Repo,
        owner: 'owner',
        repo: 'repo',
        originalUrl: 'https://github.com/owner/repo',
      };
      expect(getCloneUrl(ref)).toBe('https://github.com/owner/repo.git');
    });
  });

  describe('getHtmlUrl', () => {
    it('returns correct URL for PR', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.PR,
        owner: 'owner',
        repo: 'repo',
        prNumber: 123,
        originalUrl: 'https://github.com/owner/repo/pull/123',
      };
      expect(getHtmlUrl(ref)).toBe('https://github.com/owner/repo/pull/123');
    });

    it('returns correct URL for branch', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Branch,
        owner: 'owner',
        repo: 'repo',
        branch: 'main',
        originalUrl: 'https://github.com/owner/repo/tree/main',
      };
      expect(getHtmlUrl(ref)).toBe('https://github.com/owner/repo/tree/main');
    });

    it('returns correct URL for repo', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Repo,
        owner: 'owner',
        repo: 'repo',
        originalUrl: 'https://github.com/owner/repo',
      };
      expect(getHtmlUrl(ref)).toBe('https://github.com/owner/repo');
    });
  });

  describe('getDisplayName', () => {
    it('returns correct display name for PR', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.PR,
        owner: 'owner',
        repo: 'repo',
        prNumber: 123,
        originalUrl: 'https://github.com/owner/repo/pull/123',
      };
      expect(getDisplayName(ref)).toBe('owner/repo#123');
    });

    it('returns correct display name for branch', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Branch,
        owner: 'owner',
        repo: 'repo',
        branch: 'main',
        originalUrl: 'https://github.com/owner/repo/tree/main',
      };
      expect(getDisplayName(ref)).toBe('owner/repo:main');
    });

    it('returns correct display name for repo', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Repo,
        owner: 'owner',
        repo: 'repo',
        originalUrl: 'https://github.com/owner/repo',
      };
      expect(getDisplayName(ref)).toBe('owner/repo');
    });
  });

  describe('getSuggestedSessionName', () => {
    it('returns correct name for PR', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.PR,
        owner: 'owner',
        repo: 'my-repo',
        prNumber: 123,
        originalUrl: 'https://github.com/owner/my-repo/pull/123',
      };
      expect(getSuggestedSessionName(ref)).toBe('pr-123-my-repo');
    });

    it('returns correct name for branch', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Branch,
        owner: 'owner',
        repo: 'repo',
        branch: 'feature-branch',
        originalUrl: 'owner/repo:feature-branch',
      };
      expect(getSuggestedSessionName(ref)).toBe('repo-feature-branch');
    });

    it('sanitizes branch name with slashes', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Branch,
        owner: 'owner',
        repo: 'repo',
        branch: 'feature/test',
        originalUrl: 'owner/repo:feature/test',
      };
      expect(getSuggestedSessionName(ref)).toBe('repo-feature-test');
    });

    it('returns correct name for repo', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Repo,
        owner: 'owner',
        repo: 'my-repo',
        originalUrl: 'owner/my-repo',
      };
      expect(getSuggestedSessionName(ref)).toBe('my-repo');
    });
  });

  describe('getRepoFullName', () => {
    it('returns owner/repo format', () => {
      const ref: ParsedGitHubRef = {
        type: RefType.Repo,
        owner: 'my-owner',
        repo: 'my-repo',
        originalUrl: 'my-owner/my-repo',
      };
      expect(getRepoFullName(ref)).toBe('my-owner/my-repo');
    });
  });
});

describe('RefType enum', () => {
  it('has correct string values', () => {
    expect(RefType.PR).toBe('PR');
    expect(RefType.Branch).toBe('Branch');
    expect(RefType.Repo).toBe('Repository');
  });
});

describe('Edge cases', () => {
  it('handles whitespace in input', () => {
    const ref = parseGitHubRef('  owner/repo  ');
    expect(ref).not.toBeNull();
    expect(ref?.owner).toBe('owner');
    expect(ref?.repo).toBe('repo');
  });

  it('handles http protocol (not https)', () => {
    const ref = parseGitHubRef('http://github.com/owner/repo');
    expect(ref).not.toBeNull();
    expect(ref?.owner).toBe('owner');
    expect(ref?.repo).toBe('repo');
  });

  it('rejects invalid GitHub names starting with hyphen', () => {
    const ref = parseGitHubRef('-invalid/repo');
    expect(ref).toBeNull();
  });

  it('accepts names with dots', () => {
    const ref = parseGitHubRef('owner.name/repo.name');
    expect(ref).not.toBeNull();
    expect(ref?.owner).toBe('owner.name');
    expect(ref?.repo).toBe('repo.name');
  });

  it('accepts names with underscores', () => {
    const ref = parseGitHubRef('owner_name/repo_name');
    expect(ref).not.toBeNull();
    expect(ref?.owner).toBe('owner_name');
    expect(ref?.repo).toBe('repo_name');
  });

  it('rejects names that are too long', () => {
    const longName = 'a'.repeat(101);
    const ref = parseGitHubRef(`${longName}/repo`);
    expect(ref).toBeNull();
  });

  it('rejects names with invalid characters', () => {
    const ref = parseGitHubRef('owner$/repo');
    expect(ref).toBeNull();
  });

  it('preserves original URL', () => {
    const input = 'https://github.com/owner/repo/pull/123';
    const ref = parseGitHubRef(input);
    expect(ref?.originalUrl).toBe(input);
  });
});
