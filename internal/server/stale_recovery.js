(() => {
  const key = "_spack_sr";
  const asset = __SPACK_STALE_ASSET__;
  const limit = 32;
  let seen = [];

  try {
    const stored = JSON.parse(sessionStorage.getItem(key) || "[]");
    if (Array.isArray(stored)) {
      seen = stored.filter((value) => typeof value === "string");
    }
  } catch {
    seen = [];
  }

  if (seen.includes(asset)) {
    return;
  }

  seen.push(asset);
  if (seen.length > limit) {
    seen = seen.slice(seen.length - limit);
  }

  try {
    sessionStorage.setItem(key, JSON.stringify(seen));
  } catch {
    return;
  }
  location.reload();
})();