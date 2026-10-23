export default function HomePage() {
  return (
    <main className="min-h-screen flex flex-col items-center justify-center p-8">
      <div className="max-w-2xl text-center space-y-6">
        <h1 className="text-5xl font-bold tracking-tight">
          ApexStock Demo
        </h1>
        <p className="text-xl text-slate-400">
          Flash sale concurrency comparison platform
        </p>
        <div className="flex gap-4 justify-center mt-8">
          <div className="px-6 py-3 bg-slate-800 border border-slate-700 rounded-lg">
            <p className="text-sm text-slate-400">Baseline</p>
            <p className="text-lg font-mono">:8081</p>
          </div>
          <div className="px-6 py-3 bg-slate-800 border border-slate-700 rounded-lg">
            <p className="text-sm text-slate-400">Optimized</p>
            <p className="text-lg font-mono">:8082</p>
          </div>
        </div>
        <p className="text-sm text-slate-500 mt-8">
          Full UI implementation pending Stage 8
        </p>
      </div>
    </main>
  );
}
