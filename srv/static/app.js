// release-hub UI: copy-to-clipboard for URLs/commands.
(function () {
  function flash(btn, text) {
    var old = btn.textContent;
    btn.textContent = text;
    setTimeout(function () { btn.textContent = old; }, 1200);
  }
  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-copy]');
    if (!btn) return;
    e.preventDefault();
    var text = btn.getAttribute('data-copy');
    (navigator.clipboard ? navigator.clipboard.writeText(text) : Promise.reject())
      .then(function () { flash(btn, 'copied'); })
      .catch(function () {
        var ta = document.createElement('textarea');
        ta.value = text; document.body.appendChild(ta); ta.select();
        document.execCommand('copy'); document.body.removeChild(ta);
        flash(btn, 'copied');
      });
  });
})();
