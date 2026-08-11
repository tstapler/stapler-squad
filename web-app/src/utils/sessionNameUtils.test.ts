import { generateUniqueName } from './sessionNameUtils';

describe('generateUniqueName', () => {
  describe('when base name does not exist in the list', () => {
    it('returns the name unchanged', () => {
      const result = generateUniqueName('Base', ['Other', 'Names']);
      expect(result).toBe('Base');
    });

    it('returns the name unchanged when list has unrelated suffixed names', () => {
      const result = generateUniqueName('Base', ['Base (3)', 'Other']);
      expect(result).toBe('Base');
    });
  });

  describe('when base name already exists in the list', () => {
    it('returns "Base (2)" when "Base" is taken but "Base (2)" is not', () => {
      const result = generateUniqueName('Base', ['Base']);
      expect(result).toBe('Base (2)');
    });

    it('returns "Foo (4)" when "Foo", "Foo (2)", and "Foo (3)" are all taken', () => {
      const result = generateUniqueName('Foo', ['Foo', 'Foo (2)', 'Foo (3)']);
      expect(result).toBe('Foo (4)');
    });
  });

  describe('input with existing "(N)" suffix', () => {
    it('strips suffix and returns "Foo" when neither "Foo" nor "Foo (2)" is in the list', () => {
      // The function always strips trailing " (N)" first; if the stripped base is
      // not taken it returns the stripped form, not the original suffixed input.
      const result = generateUniqueName('Foo (2)', ['Other', 'Names']);
      expect(result).toBe('Foo');
    });

    it('returns "Foo (2)" when stripped "Foo" is taken but "Foo (2)" is not', () => {
      // Input "Foo (2)" strips to "Foo"; "Foo" is taken, but original "Foo (2)" is not
      const result = generateUniqueName('Foo (2)', ['Foo']);
      expect(result).toBe('Foo (2)');
    });

    it('returns "Foo (4)" when "Foo (2)" input and "Foo", "Foo (2)", "Foo (3)" are taken', () => {
      // Stripped base "Foo" is taken; original "Foo (2)" also taken; increments to (4)
      const result = generateUniqueName('Foo (2)', ['Foo', 'Foo (2)', 'Foo (3)']);
      expect(result).toBe('Foo (4)');
    });
  });

  describe('empty existingNames array', () => {
    it('returns name unchanged when list is empty', () => {
      const result = generateUniqueName('My Session', []);
      expect(result).toBe('My Session');
    });
  });

  describe('empty string input', () => {
    it('returns empty string for empty input', () => {
      const result = generateUniqueName('', ['Existing']);
      expect(result).toBe('');
    });

    it('returns whitespace-only string unchanged (guard returns early)', () => {
      const result = generateUniqueName('   ', ['Existing']);
      expect(result).toBe('   ');
    });
  });

  describe('when counters 2 through 5 are all taken', () => {
    it('returns counter 6 when 2 through 5 are occupied', () => {
      const result = generateUniqueName('Task', [
        'Task',
        'Task (2)',
        'Task (3)',
        'Task (4)',
        'Task (5)',
      ]);
      expect(result).toBe('Task (6)');
    });
  });

  describe('additional edge cases', () => {
    it('handles names with spaces correctly', () => {
      const result = generateUniqueName('My Feature Branch', ['My Feature Branch']);
      expect(result).toBe('My Feature Branch (2)');
    });

    it('does not confuse "(N)" in the middle of a name with a trailing suffix', () => {
      // "(2)" is in middle not end; the name should be treated as a plain name
      const result = generateUniqueName('Session (2) Test', ['Session (2) Test']);
      expect(result).toBe('Session (2) Test (2)');
    });

    it('finds next available slot when there are gaps', () => {
      // Counter starts at 2; "Foo (2)" missing so it should return "Foo (2)"
      const result = generateUniqueName('Foo', ['Foo', 'Foo (3)', 'Foo (4)']);
      expect(result).toBe('Foo (2)');
    });
  });
});
