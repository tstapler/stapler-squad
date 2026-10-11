import { ForwardEchoGuard, contentSignature, longestSignaturePrefixContained } from '../ForwardEchoGuard';

describe('contentSignature', () => {
  it('strips CSI/OSC sequences and all whitespace', () => {
    expect(contentSignature('\x1b[31mhello\x1b[0m  wor\nld')).toBe('helloworld');
    expect(contentSignature('\x1b]0;title\x07abc')).toBe('abc');
    expect(contentSignature('a\x1b[!pb')).toBe('ab'); // DECSTR: non-letter final byte
  });
});

describe('longestSignaturePrefixContained', () => {
  it('returns the longest contained prefix length', () => {
    expect(longestSignaturePrefixContained('abcdef', 'xxabcdyy')).toBe(4);
    expect(longestSignaturePrefixContained('abc', 'xyz')).toBe(0);
    expect(longestSignaturePrefixContained('abc', 'zabcz')).toBe(3);
    expect(longestSignaturePrefixContained('', 'abc')).toBe(0);
  });
});

describe('ForwardEchoGuard', () => {
  it('is not armed before anything is recorded', () => {
    expect(new ForwardEchoGuard().isSelfEcho('anything')).toBe(false);
  });

  it('treats a single-frame redraw of the recorded content as an echo, ignoring ANSI/whitespace', () => {
    const guard = new ForwardEchoGuard();
    guard.record('line one\nline two');
    expect(guard.isSelfEcho('\x1b[1mline one\x1b[0m   line two')).toBe(true);
  });

  it('accumulates an echo split across frames', () => {
    const guard = new ForwardEchoGuard();
    guard.record('alpha bravo charlie delta');
    expect(guard.isSelfEcho('alpha bravo')).toBe(true);
    expect(guard.isSelfEcho('charlie delta')).toBe(true);
  });

  it('classifies unrelated content as a genuine live resume', () => {
    const guard = new ForwardEchoGuard();
    guard.record('alpha bravo charlie');
    expect(guard.isSelfEcho('completely different output')).toBe(false);
  });

  it('does not let a stale partial match misclassify later unrelated output', () => {
    const guard = new ForwardEchoGuard();
    guard.record('alpha bravo charlie delta');
    expect(guard.isSelfEcho('alpha bravo')).toBe(true);
    expect(guard.isSelfEcho('unrelated')).toBe(false);
  });

  it('treats whitespace/escape-only frames as mid-echo', () => {
    const guard = new ForwardEchoGuard();
    guard.record('alpha');
    expect(guard.isSelfEcho('\x1b[2J \n')).toBe(true);
  });

  it('disarms on empty record() and on clear()', () => {
    const guard = new ForwardEchoGuard();
    guard.record('');
    expect(guard.isSelfEcho('alpha')).toBe(false);

    guard.record('alpha');
    guard.clear();
    expect(guard.isSelfEcho('alpha')).toBe(false);
  });
});
