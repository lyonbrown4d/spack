(() => {
  const k = "_spack_sr";
  if (sessionStorage.getItem(k)) {
    sessionStorage.removeItem(k);
    return;
  }
  try {
    sessionStorage.setItem(k, "1");
  } catch (e) {}
  location.reload();
})();
