# The Dancing Pony: a two-minute tour of the live API.
#
#   hurl --test try-it.hurl              (https://hurl.dev; or, with Docker:)
#   docker run --rm -v "$PWD:/w" -w /w ghcr.io/orange-opensource/hurl:latest --test try-it.hurl
#
# Every request checks its own response, so a green run means each feature
# works on production right now. It registers a new customer each run and
# removes the dish it creates. Running it a few times is fine; the login
# throttle at the end allows that before it starts slowing your IP down.

# 1. Health: the API and its database
GET https://silson.dev/readyz
HTTP 200
[Asserts]
jsonpath "$.status" == "ok"
jsonpath "$.database" == "ok"


# 2. Register a customer (a fresh email every run)
POST https://silson.dev/auth/register
{
  "name": "Reviewer",
  "email": "reviewer-{{newUuid}}@example.com",
  "password": "a long enough password"
}
HTTP 201
[Captures]
email: jsonpath "$.email"
[Asserts]
jsonpath "$.id" exists
jsonpath "$.password" not exists


# ...and log in: the response carries an opaque session token
POST https://silson.dev/auth/login
{
  "email": "{{email}}",
  "password": "a long enough password"
}
HTTP 200
[Captures]
me: jsonpath "$.token"
[Asserts]
jsonpath "$.token_type" == "Bearer"


GET https://silson.dev/auth/me
Authorization: Bearer {{me}}
HTTP 200
[Asserts]
jsonpath "$.email" == "{{email}}"


# 3. Everything needs a login; errors are RFC 9457 problem details
GET https://silson.dev/restaurants/orc-shack/dishes
HTTP 401
[Asserts]
header "Content-Type" contains "application/problem+json"
header "WWW-Authenticate" contains "Bearer"


# 4. The Orc Shack's menu (prices in minor units, cents of ZAR)
GET https://silson.dev/restaurants/orc-shack/dishes
Authorization: Bearer {{me}}
HTTP 200
[Asserts]
jsonpath "$" count >= 2
jsonpath "$[*].price.currency" contains "ZAR"


# 5. Search, typo-tolerant: "mushrom" still finds Mushroom Pie
GET https://silson.dev/restaurants/orc-shack/dishes
Authorization: Bearer {{me}}
[QueryStringParams]
q: mushrom
HTTP 200
[Asserts]
jsonpath "$[0].name" == "Mushroom Pie"


# 6. Filter, sort and page: the next page's URL is in the Link header
GET https://silson.dev/restaurants/orc-shack/dishes
Authorization: Bearer {{me}}
[QueryStringParams]
max_price: 20000
sort: -price
limit: 1
HTTP 200
[Asserts]
jsonpath "$" count == 1
jsonpath "$[0].name" == "Mushroom Pie"
header "Link" contains "rel=\"next\""


# 7. View one dish...
GET https://silson.dev/restaurants/orc-shack/dishes
Authorization: Bearer {{me}}
[QueryStringParams]
q: seed cake
HTTP 200
[Captures]
dish: jsonpath "$[0].id"


GET https://silson.dev/restaurants/orc-shack/dishes/{{dish}}
Authorization: Bearer {{me}}
HTTP 200
[Asserts]
jsonpath "$.name" == "Seed Cake"
jsonpath "$.rating.count" exists


# ...and review it: one review per customer per dish, scored for sentiment
PUT https://silson.dev/restaurants/orc-shack/dishes/{{dish}}/reviews/me
Authorization: Bearer {{me}}
{
  "rating": 4,
  "body": "Lovely, if a little dry. Better than second breakfast."
}
HTTP 201
[Asserts]
jsonpath "$.rating" == 4
jsonpath "$.sentiment.label" == "positive"
jsonpath "$.sentiment.model" == "wordlist-v1"


# A rating outside 1-5 is refused
PUT https://silson.dev/restaurants/orc-shack/dishes/{{dish}}/reviews/me
Authorization: Bearer {{me}}
{
  "rating": 6
}
HTTP 400


# 8. A customer can't add dishes
POST https://silson.dev/restaurants/orc-shack/dishes
Authorization: Bearer {{me}}
{
  "name": "Po-tay-toes",
  "description": "Boil 'em, mash 'em, stick 'em in a stew.",
  "price": { "amount": 3000, "currency": "ZAR" },
  "image": "https://example.com/potatoes.jpg"
}
HTTP 403
[Asserts]
jsonpath "$.detail" contains "customer"


# 9. The owner can: log in, create, update
POST https://silson.dev/auth/login
{
  "email": "owner@orc-shack.example",
  "password": "pony-demo-password"
}
HTTP 200
[Captures]
owner: jsonpath "$.token"


POST https://silson.dev/restaurants/orc-shack/dishes
Authorization: Bearer {{owner}}
{
  "name": "Reviewer Stew",
  "description": "Rabbit, a brace of coneys.",
  "price": { "amount": 8500, "currency": "ZAR" },
  "image": "https://example.com/stew.jpg"
}
HTTP 201
[Captures]
new: jsonpath "$.id"


PUT https://silson.dev/restaurants/orc-shack/dishes/{{new}}
Authorization: Bearer {{owner}}
{
  "name": "Reviewer Stew",
  "description": "Now with taters.",
  "price": { "amount": 9000, "currency": "ZAR" },
  "image": "https://example.com/stew.jpg"
}
HTTP 200
[Asserts]
jsonpath "$.description" == "Now with taters."
jsonpath "$.price.amount" == 9000


# 10. Validation: bad input is refused with a reason, never guessed at
POST https://silson.dev/restaurants/orc-shack/dishes
Authorization: Bearer {{owner}}
{
  "name": "Reviewer Stew",
  "description": "Priced in the wrong currency.",
  "price": { "amount": 9000, "currency": "USD" },
  "image": "https://example.com/stew.jpg"
}
HTTP 400


POST https://silson.dev/restaurants/orc-shack/dishes
Authorization: Bearer {{owner}}
{
  "name": "Reviewer Stew",
  "prise": 9000
}
HTTP 400
[Asserts]
jsonpath "$.detail" contains "prise"


# 11. Multi-tenancy: another restaurant's owner can't see or delete it
POST https://silson.dev/auth/login
{
  "email": "owner@green-dragon.example",
  "password": "pony-demo-password"
}
HTTP 200
[Captures]
dragon: jsonpath "$.token"


GET https://silson.dev/restaurants/green-dragon/dishes/{{new}}
Authorization: Bearer {{dragon}}
HTTP 404


DELETE https://silson.dev/restaurants/orc-shack/dishes/{{new}}
Authorization: Bearer {{dragon}}
HTTP 403


# 12. The owner deletes it, and it's gone
DELETE https://silson.dev/restaurants/orc-shack/dishes/{{new}}
Authorization: Bearer {{owner}}
HTTP 204


GET https://silson.dev/restaurants/orc-shack/dishes/{{new}}
Authorization: Bearer {{owner}}
HTTP 404


# 13. Logout revokes the token at once (sessions, not JWTs)
POST https://silson.dev/auth/logout
Authorization: Bearer {{me}}
HTTP 204


GET https://silson.dev/auth/me
Authorization: Bearer {{me}}
HTTP 401


# 14. Brute force: wrong passwords are slowed down, never locked out.
# A few free attempts, then 429 with Retry-After.
POST https://silson.dev/auth/login
{ "email": "{{email}}", "password": "wrong" }
HTTP 401

POST https://silson.dev/auth/login
{ "email": "{{email}}", "password": "wrong" }
HTTP 401

POST https://silson.dev/auth/login
{ "email": "{{email}}", "password": "wrong" }
HTTP 401

POST https://silson.dev/auth/login
{ "email": "{{email}}", "password": "wrong" }
HTTP 401

POST https://silson.dev/auth/login
{ "email": "{{email}}", "password": "wrong" }
HTTP 401

POST https://silson.dev/auth/login
{ "email": "{{email}}", "password": "wrong" }
HTTP 401

POST https://silson.dev/auth/login
{ "email": "{{email}}", "password": "wrong" }
HTTP 429
[Asserts]
header "Retry-After" exists
