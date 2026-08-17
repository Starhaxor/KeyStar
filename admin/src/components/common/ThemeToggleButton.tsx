"use client";
import React, { useState } from "react";
import { useTheme, type Theme } from "../../context/ThemeContext";
import { Dropdown } from "../ui/dropdown/Dropdown";
import { DropdownItem } from "../ui/dropdown/DropdownItem";

function SunIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 20 20"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M10 3.125C10.3452 3.125 10.625 2.84518 10.625 2.5V1.25C10.625 0.904822 10.3452 0.625 10 0.625C9.65482 0.625 9.375 0.904822 9.375 1.25V2.5C9.375 2.84518 9.65482 3.125 10 3.125Z"
        fill="currentColor"
      />
      <path
        d="M10 16.875C9.65482 16.875 9.375 17.1548 9.375 17.5V18.75C9.375 19.0952 9.65482 19.375 10 19.375C10.3452 19.375 10.625 19.0952 10.625 18.75V17.5C10.625 17.1548 10.3452 16.875 10 16.875Z"
        fill="currentColor"
      />
      <path
        d="M3.5717 5.35798C3.81474 5.101 4.06008 5.11299 4.30533 5.35831L5.17883 6.23181C5.41412 6.46711 5.42614 6.71236 5.16914 6.9554C4.9261 7.2124 4.68076 7.20041 4.43551 6.9551L3.56201 6.0816C3.32672 5.8463 3.3147 5.60105 3.5717 5.35798Z"
        fill="currentColor"
      />
      <path
        d="M16.4282 5.35798C16.1852 5.101 15.9398 5.11299 15.6946 5.35831L14.8211 6.23181C14.5858 6.46711 14.5738 6.71236 14.8308 6.9554C15.0738 7.2124 15.3192 7.20041 15.5644 6.9551L16.4379 6.0816C16.6732 5.8463 16.6852 5.60105 16.4282 5.35798Z"
        fill="currentColor"
      />
      <path
        d="M3.125 10C3.125 10.3452 2.84518 10.625 2.5 10.625H1.25C0.904822 10.625 0.625 10.3452 0.625 10C0.625 9.65482 0.904822 9.375 1.25 9.375H2.5C2.84518 9.375 3.125 9.65482 3.125 10Z"
        fill="currentColor"
      />
      <path
        d="M19.375 10C19.375 10.3452 19.0952 10.625 18.75 10.625H17.5C17.1548 10.625 16.875 10.3452 16.875 10C16.875 9.65482 17.1548 9.375 17.5 9.375H18.75C19.0952 9.375 19.375 9.65482 19.375 10Z"
        fill="currentColor"
      />
      <path
        d="M5.35798 16.4282C5.101 16.1852 5.11299 15.9398 5.35831 15.6946L6.23181 14.8211C6.46711 14.5858 6.71236 14.5738 6.9554 14.8308C7.2124 15.0738 7.20041 15.3192 6.9551 15.5644L6.0816 16.4379C5.8463 16.6732 5.60105 16.6852 5.35798 16.4282Z"
        fill="currentColor"
      />
      <path
        d="M14.642 16.4282C14.385 16.1852 14.373 15.9398 14.6183 15.6946L15.4918 14.8211C15.7271 14.5858 15.9723 14.5738 16.2154 14.8308C16.4724 15.0738 16.4604 15.3192 16.2151 15.5644L15.3416 16.4379C15.1063 16.6732 14.861 16.6852 14.642 16.4282Z"
        fill="currentColor"
      />
      <path
        d="M10 5.625C12.4162 5.625 14.375 7.58375 14.375 10C14.375 12.4162 12.4162 14.375 10 14.375C7.58375 14.375 5.625 12.4162 5.625 10C5.625 7.58375 7.58375 5.625 10 5.625Z"
        fill="currentColor"
      />
    </svg>
  );
}

function MoonIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 20 20"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M17.4547 11.97L18.1799 12.1611C18.265 11.8383 18.1265 11.4982 17.8401 11.3266C17.5538 11.1551 17.1885 11.1934 16.944 11.4207L17.4547 11.97ZM8.0306 2.5459L8.57989 3.05657C8.80718 2.81209 8.84554 2.44682 8.67398 2.16046C8.50243 1.8741 8.16227 1.73559 7.83948 1.82066L8.0306 2.5459ZM12.9154 13.0035C9.64678 13.0035 6.99707 10.3538 6.99707 7.08524H5.49707C5.49707 11.1823 8.81835 14.5035 12.9154 14.5035V13.0035ZM16.944 11.4207C15.8869 12.4035 14.4721 13.0035 12.9154 13.0035V14.5035C14.8657 14.5035 16.6418 13.7499 17.9654 12.5193L16.944 11.4207ZM16.7295 11.7789C15.9437 14.7607 13.2277 16.9586 10.0003 16.9586V18.4586C13.9257 18.4586 17.2249 15.7853 18.1799 12.1611L16.7295 11.7789ZM10.0003 16.9586C6.15734 16.9586 3.04199 13.8433 3.04199 10.0003H1.54199C1.54199 14.6717 5.32892 18.4586 10.0003 18.4586V16.9586ZM3.04199 10.0003C3.04199 6.77289 5.23988 4.05695 8.22173 3.27114L7.83948 1.82066C4.21532 2.77574 1.54199 6.07486 1.54199 10.0003H3.04199ZM6.99707 7.08524C6.99707 5.52854 7.5971 4.11366 8.57989 3.05657L7.48132 2.03522C6.25073 3.35885 5.49707 5.13487 5.49707 7.08524H6.99707Z"
        fill="currentColor"
      />
    </svg>
  );
}

function MonitorIcon() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 20 20"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M2.5 3.125C1.67157 3.125 1 3.79657 1 4.625V12.875C1 13.7034 1.67157 14.375 2.5 14.375H9.375V15.625H6.875C6.52982 15.625 6.25 15.9048 6.25 16.25C6.25 16.5952 6.52982 16.875 6.875 16.875H13.125C13.4702 16.875 13.75 16.5952 13.75 16.25C13.75 15.9048 13.4702 15.625 13.125 15.625H10.625V14.375H17.5C18.3284 14.375 19 13.7034 19 12.875V4.625C19 3.79657 18.3284 3.125 17.5 3.125H2.5ZM2.5 4.375H17.5C17.6381 4.375 17.75 4.48693 17.75 4.625V12.875C17.75 13.0131 17.6381 13.125 17.5 13.125H2.5C2.36193 13.125 2.25 13.0131 2.25 12.875V4.625C2.25 4.48693 2.36193 4.375 2.5 4.375Z"
        fill="currentColor"
      />
    </svg>
  );
}

const options: { value: Theme; label: string; icon: React.ReactNode }[] = [
  { value: "light", label: "Light", icon: <SunIcon /> },
  { value: "dark", label: "Dark", icon: <MoonIcon /> },
  { value: "system", label: "System", icon: <MonitorIcon /> },
];

export const ThemeToggleButton: React.FC = () => {
  const { theme, setTheme } = useTheme();
  const [isOpen, setIsOpen] = useState(false);

  const current = options.find((option) => option.value === theme) ?? options[2];

  return (
    <div className="relative">
      <button
        className="dropdown-toggle relative flex h-11 w-11 items-center justify-center rounded-full border border-gray-200 bg-white text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-400 dark:hover:bg-gray-800 dark:hover:text-white"
        onClick={() => setIsOpen((prev) => !prev)}
        aria-label="Toggle theme"
      >
        {current.icon}
      </button>
      <Dropdown isOpen={isOpen} onClose={() => setIsOpen(false)}>
        <div className="w-40 p-2">
          <p className="px-3 py-1.5 text-xs font-medium uppercase text-gray-400">
            Theme
          </p>
          {options.map((option) => (
            <DropdownItem
              key={option.value}
              onClick={() => {
                setTheme(option.value);
                setIsOpen(false);
              }}
              baseClassName="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm"
              className={
                theme === option.value
                  ? "bg-brand-50 font-medium text-brand-600 dark:bg-brand-500/10 dark:text-brand-400"
                  : "text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-white/5"
              }
            >
              {option.icon}
              <span className="flex-1 text-left">{option.label}</span>
              {theme === option.value && (
                <svg
                  width="16"
                  height="16"
                  viewBox="0 0 20 20"
                  fill="none"
                  xmlns="http://www.w3.org/2000/svg"
                  className="text-brand-500 dark:text-brand-400"
                >
                  <path
                    d="M16.7045 5.2955C17.0986 5.68963 17.0986 6.32537 16.7045 6.7195L8.3695 15.0545C7.97537 15.4486 7.33963 15.4486 6.9455 15.0545L3.2955 11.4045C2.90137 11.0104 2.90137 10.3746 3.2955 9.9805C3.68963 9.58637 4.32537 9.58637 4.7195 9.9805L7.6575 12.9185L15.2805 5.2955C15.6746 4.90137 16.3104 4.90137 16.7045 5.2955Z"
                    fill="currentColor"
                  />
                </svg>
              )}
            </DropdownItem>
          ))}
        </div>
      </Dropdown>
    </div>
  );
};
