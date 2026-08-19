#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::collections::HashMap;
use serde::{Deserialize, Serialize};

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct HttpRequest {
    url: String,
    method: String,
    headers: HashMap<String, String>,
    body: Option<String>,
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct HttpResponse {
    status: u16,
    body: String,
    content_type: String,
}

struct HttpClient(reqwest::Client);

#[tauri::command]
async fn http_request(request: HttpRequest, client: tauri::State<'_, HttpClient>) -> Result<HttpResponse, String> {
    let url = reqwest::Url::parse(&request.url).map_err(|_| "invalid server URL".to_string())?;
    let local = matches!(url.host_str(), Some("localhost" | "127.0.0.1" | "::1"));
    if url.scheme() != "https" && !(local && url.scheme() == "http") {
        return Err("HTTPS is required outside localhost".into());
    }
    let method = reqwest::Method::from_bytes(request.method.as_bytes()).map_err(|_| "invalid HTTP method".to_string())?;
    let mut builder = client.0.request(method, url);
    for (name, value) in request.headers {
        let header_name = reqwest::header::HeaderName::from_bytes(name.as_bytes()).map_err(|_| "invalid header".to_string())?;
        let header_value = reqwest::header::HeaderValue::from_str(&value).map_err(|_| "invalid header value".to_string())?;
        builder = builder.header(header_name, header_value);
    }
    if let Some(body) = request.body { builder = builder.body(body); }
    let response = builder.send().await.map_err(|error| format!("server request failed: {error}"))?;
    let status = response.status().as_u16();
    let content_type = response.headers().get(reqwest::header::CONTENT_TYPE).and_then(|value| value.to_str().ok()).unwrap_or("application/json").to_string();
    let body = response.text().await.map_err(|_| "unable to read server response".to_string())?;
    Ok(HttpResponse { status, body, content_type })
}

fn main() {
    let client = reqwest::Client::builder().cookie_store(true).build().expect("failed to initialize HTTP client");
    tauri::Builder::default()
        .manage(HttpClient(client))
        .invoke_handler(tauri::generate_handler![http_request])
        .run(tauri::generate_context!())
        .expect("failed to run LicenseHub Console");
}
