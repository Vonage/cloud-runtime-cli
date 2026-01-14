module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    // Commit types allowed
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

    // Relaxed rules - disable strict length limits
    'body-max-line-length': [0],         // Disable body line length limit
    'header-max-length': [0],            // Disable header length limit
    'footer-max-line-length': [0],       // Disable footer line length limit
    'subject-case': [0],                 // Allow any case in subject
    'subject-full-stop': [0],            // Allow trailing period in subject
    'body-leading-blank': [1, 'always'], // Warn (not error) if no blank line before body
  },
};
