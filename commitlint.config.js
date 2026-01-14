module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'type-enum': [
      2,
      'always',
      [
        'feat',     // New feature
        'fix',      // Bug fix
        'docs',     // Documentation only
        'style',    // Formatting, missing semicolons, etc.
        'refactor', // Code change that neither fixes a bug nor adds a feature
        'perf',     // Performance improvement
        'test',     // Adding missing tests
        'build',    // Changes to build process
        'ci',       // CI configuration changes
        'chore',    // Other changes that don't modify src or test files
        'revert',   // Reverts a previous commit
      ],
    ],
    'subject-case': [0], // Disable subject case check (allow any case)
  },
};
