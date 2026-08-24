import PageBreadcrumb from "@/components/common/PageBreadCrumb";
import OnboardingWizard from "@/components/onboarding/OnboardingWizard";

export default function OnboardingPage() {
  return (
    <>
      <PageBreadcrumb pageTitle="Application onboarding" />
      <OnboardingWizard />
    </>
  );
}
