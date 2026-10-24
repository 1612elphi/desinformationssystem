export default {
  extends: ['stylelint-config-standard'],
  rules: {
    // carbon and the ported styles use bem class names
    'selector-class-pattern': null,
    // the accent section at the end of app.css overrides earlier rules
    'no-duplicate-selectors': null,
    'no-descending-specificity': null,
  },
};
