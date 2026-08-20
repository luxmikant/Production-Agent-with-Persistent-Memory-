import "./globals.css";

export const metadata = {
  title: "StillMind",
  description: "A meditation agent with persistent memory on CockroachDB and AWS.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <div className="banner">
          <b>StillMind</b> is a non-clinical lifestyle coach. Not medical advice. If you are in crisis, contact local emergency services.
        </div>
        {children}
      </body>
    </html>
  );
}
