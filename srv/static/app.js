// release-hub UI: copy-to-clipboard + Play connection check.
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

  // Destructive forms (release delete) ask for confirmation.
  document.addEventListener('submit', function (e) {
    var f = e.target.closest('form[data-confirm]');
    if (f && !window.confirm(f.getAttribute('data-confirm'))) e.preventDefault();
  });

  // Play preflight: hit the check endpoint, show ok/detail inline.
  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-play-check]');
    if (!btn) return;
    e.preventDefault();
    var box = btn.closest('.playinfo, .playsetup').parentElement.querySelector('[data-play-result]');
    if (!box) return;
    var out = box.querySelector('code');
    box.hidden = false;
    out.textContent = 'checking…';
    btn.disabled = true;
    fetch(btn.getAttribute('data-play-check'), { headers: { 'Accept': 'application/json' } })
      .then(function (r) { return r.json(); })
      .then(function (j) {
        out.textContent = (j.ok ? '✓ ' : '✗ ') + j.detail;
        out.style.color = j.ok ? 'var(--green)' : '#c33';
      })
      .catch(function (err) {
        out.textContent = '✗ request failed: ' + err;
        out.style.color = '#c33';
      })
      .finally(function () { btn.disabled = false; });
  });
})();
