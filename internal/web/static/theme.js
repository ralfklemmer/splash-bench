// Applies a stored light/dark choice before the page paints; without one
// the page follows the system setting.
(function () {
  try {
    var theme = localStorage.getItem('theme');
    if (theme === 'light' || theme === 'dark') document.documentElement.dataset.theme = theme;
  } catch (e) { /* storage blocked: follow the system */ }
})();
