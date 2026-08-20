#include "keystar/client.hpp"
#include "keystar/json_parser.hpp"

namespace keystar {

// The auth module is integrated into the Client class. This file exists as
// a placeholder for potential future separation into an Auth class that
// the Client delegates to. Currently, Client::login() handles the full
// auth flow including device verification.

// Future API sketch:
//
//   class Auth {
//   public:
//       ApiResponse<SessionResult> login(const std::string& email,
//                                        const std::string& password);
//       ApiResponse<SessionResult> refresh();
//       bool logout();
//       ApiResponse<UserProfile> me();
//   };
//
// The Client would hold an Auth instance and forward calls to it.

}  // namespace keystar
