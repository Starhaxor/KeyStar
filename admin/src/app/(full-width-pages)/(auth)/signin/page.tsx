import SignInForm from "@/components/auth/SignInForm";
import { Metadata } from "next";

export const metadata: Metadata = {
  title: "Sign In | KeyStar Admin",
  description: "Sign in to the StarLoader admin console.",
};

export default function SignIn() {
  return <SignInForm />;
}
