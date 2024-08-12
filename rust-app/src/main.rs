#![feature(impl_trait_in_fn_trait_return)]

use std::{ops::Deref, time::Duration};

use sqlx::{postgres::PgPoolOptions, PgPool};
use tokio::{
    sync::broadcast::{channel, Sender},
    time::timeout,
};

use actix_web::{get, middleware::Logger, web, App, HttpResponse, HttpServer, Responder};
use env_logger::Env;

#[derive(Debug, PartialEq, Eq, Copy, Clone)]
enum Message {
    Open,
}

struct Gate {
    tx: Sender<Message>,
}

#[get("/open")]
async fn open(state: web::Data<Gate>) -> impl Responder {
    match state.tx.send(Message::Open) {
        Ok(_) => HttpResponse::Ok(),
        Err(_) => HttpResponse::NoContent(),
    }
}

#[get("/gate")]
async fn gate(state: web::Data<Gate>) -> impl Responder {
    match timeout(Duration::from_millis(60_000), state.tx.subscribe().recv()).await {
        Ok(Ok(Message::Open)) => HttpResponse::Ok(),
        _ => HttpResponse::RequestTimeout(),
    }
}

async fn init_db() -> PgPool {
    match PgPoolOptions::new()
        .max_connections(5)
        .connect("postgres://postgres:postgres@localhost/gate")
        .await
    {
        Ok(pool) => pool,
        Err(err) => panic!("Failed to initialize database: {err}"),
    }
}

#[actix_web::main]
async fn main() -> std::io::Result<()> {
    let database = web::Data::new(init_db().await);

    let user = sqlx::query!("select * from users")
        .fetch_one(&**database)
        .await
        .unwrap();

    dbg!(user);

    let (tx, _) = channel(1);
    let state = web::Data::new(Gate { tx });

    env_logger::init_from_env(Env::default().default_filter_or("debug"));

    HttpServer::new(move || {
        App::new()
            .wrap(Logger::default())
            .app_data(database.clone())
            .app_data(state.clone())
            .service(open)
            .service(gate)
    })
    .bind(("127.0.0.1", 8080))?
    .run()
    .await
}
