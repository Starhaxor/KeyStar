#pragma once

// KeyStar C++ SDK — single-include header.
//
// Usage:
//   #include <keystar/keystar.hpp>
//
//   keystar::Client client({
//       .application_id = "019c...",
//       .publishable_key = "ks_pk_live_...",
//       .base_url = "https://api.keystar.dev"
//   });
//
//   auto result = client.login(email, password);

#include "error.hpp"
#include "types.hpp"
#include "transport.hpp"
#include "device_identity.hpp"
#include "token_store.hpp"
#include "client.hpp"
