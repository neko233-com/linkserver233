function getOriginBaseURL(env) {
  const value = String(env.ORIGIN_BASE_URL || "").trim().replace(/\/+$/, "");
  if (!value) {
    throw new Error("Missing ORIGIN_BASE_URL worker secret/var");
  }

  const url = new URL(value);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("ORIGIN_BASE_URL must use http or https");
  }

  return url;
}

function buildOriginURL(requestURL, originBaseURL) {
  return new URL(requestURL.pathname + requestURL.search, originBaseURL);
}

function buildForwardHeaders(request, env) {
  const headers = new Headers(request.headers);
  headers.delete("host");
  headers.delete("content-length");

  const clientIP = request.headers.get("CF-Connecting-IP");
  if (clientIP) {
    headers.set("X-Forwarded-For", clientIP);
  }

  headers.set("X-Forwarded-Proto", "https");
  headers.set("X-Forwarded-Host", new URL(request.url).host);

  const originAuthBearer = String(env.ORIGIN_AUTH_BEARER || "").trim();
  if (originAuthBearer) {
    headers.set("Authorization", `Bearer ${originAuthBearer}`);
  }

  return headers;
}

export default {
  async fetch(request, env) {
    const requestURL = new URL(request.url);
    const originBaseURL = getOriginBaseURL(env);
    const originURL = buildOriginURL(requestURL, originBaseURL);

    const init = {
      method: request.method,
      headers: buildForwardHeaders(request, env),
      redirect: "manual"
    };

    if (request.method !== "GET" && request.method !== "HEAD") {
      init.body = request.body;
      init.duplex = "half";
    }

    return fetch(originURL, init);
  }
};
