import React, { useEffect, useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { ReportedMessage, useAppStore } from '../store/useAppStore';
import { apiService } from '../services/api';
import { Sidebar } from '@/components/layout/Sidebar';
import { Check, X, AlertTriangle, MessageSquare, ImageIcon } from 'lucide-react';
import DOMPurify from 'dompurify';

// Parse <HEAD>title</HEAD><BODY>body</BODY> format
function parseContent(raw: string): { title: string; body: string } {
  const headMatch = raw.match(/<HEAD>([\s\S]*?)<\/HEAD>/i);
  const bodyMatch = raw.match(/<BODY>([\s\S]*?)<\/BODY>/i);
  return {
    title: headMatch?.[1] ?? '',
    body: bodyMatch?.[1] ?? raw,
  };
}

// Extract image srcs from HTML, returning sanitized text HTML and src list
function extractImages(html: string): { textHtml: string; imgSrcs: string[] } {
  const safe = DOMPurify.sanitize(html, {
    ADD_TAGS: ['img'],
    ADD_ATTR: ['src', 'alt', 'class', 'style'],
    ALLOW_DATA_ATTR: true,
    FORCE_BODY: true,
  });
  const div = document.createElement('div');
  div.innerHTML = safe;
  const srcs: string[] = [];
  div.querySelectorAll('img').forEach(img => {
    srcs.push(img.src);
    img.remove();
  });
  return { textHtml: div.innerHTML, imgSrcs: srcs };
}

export const MsgReports: React.FC = () => {
  const { user } = useAppStore();
  const [reports, setReports] = useState<ReportedMessage[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [lightboxSrc, setLightboxSrc] = useState<string | null>(null);

  useEffect(() => {
    loadReports();
  }, []);

  const loadReports = async () => {
    setIsLoading(true);
    try {
      const moderationReports = await apiService.getMessageReports("1");
      setReports(moderationReports);
    } catch (error) {
      console.error('Failed to load moderation reports:', error);
    } finally {
      setIsLoading(false);
    }
  };

  const handleModerate = async (sign: string, action: 'approve' | 'reject') => {
    try {
      // Use moderateBySign to pass the sign string directly, avoiding
      // JS→Go MsgCert struct mapping issues where sign could arrive empty.
      const report = reports.find(r => r.sign === sign);
      const isManualMod = report?.type === 'manual_mod';
      const baseValue = action === 'approve' ? 0 : 1;
      const moderationValue = isManualMod ? (baseValue === 0 ? 1 : 0) : baseValue;
      await apiService.moderateBySign(sign, moderationValue);
      setReports(prev => prev.filter(r => r.sign !== sign));
    } catch (error) {
      console.error('Failed to moderate message:', error);
    }
  };

  if (user?.role !== 'moderator') {
    return (
      <div className='flex flex-row'>
        <div className='w-[19.4%]'><Sidebar /></div>
        <div className="flex-1 flex items-center justify-center bg-libr-primary">
          <motion.div initial={{ opacity: 0, scale: 0.9 }} animate={{ opacity: 1, scale: 1 }} className="text-center">
            <div className="w-20 h-20 bg-red-500/20 rounded-2xl flex items-center justify-center mx-auto mb-4">
              <X className="w-10 h-10 text-red-500" />
            </div>
            <h2 className="text-xl font-semibold text-foreground mb-2">Access Denied</h2>
            <p className="text-muted-foreground">You need moderator privileges to access this page</p>
          </motion.div>
        </div>
      </div>
    );
  }

  return (
    <>
      <div className='flex flex-row'>
        <div className='w-[19.4%]'><Sidebar /></div>
        <div className="flex-1 flex flex-col w-full bg-libr-primary h-screen">
          <div className="flex-1 overflow-y-auto">
            <div className="pt-6 pb-24">
              <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} className="max-w-7xl mx-4">

                {/* Header */}
                <div className="mb-8 w-full">
                  <div className="flex items-center space-x-3 mb-4">
                    <div className="w-12 h-12 bg-libr-accent2/20 rounded-xl flex items-center justify-center">
                      <MessageSquare className="w-6 h-6 text-libr-accent2" />
                    </div>
                    <div>
                      <h1 className="text-2xl font-bold text-foreground">Message Reports</h1>
                      <p className="text-muted-foreground">These messages have been reported by users and require moderation.</p>
                    </div>
                  </div>
                  <button
                    onClick={loadReports}
                    className="px-3 py-2 bg-muted/30 border border-border/50 text-base rounded-lg text-foreground hover:bg-muted/40 transition-colors"
                  >
                    Refresh
                  </button>
                </div>

                {/* Reports */}
                {isLoading ? (
                  <div className="flex items-center justify-center py-12">
                    <motion.div
                      animate={{ rotate: 360 }}
                      transition={{ duration: 1, repeat: Infinity, ease: "linear" }}
                      className="w-8 h-8 border-2 border-libr-accent2 border-t-transparent rounded-full"
                    />
                  </div>
                ) : reports.length === 0 ? (
                  <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} className="text-center py-12">
                    <div className="w-16 h-16 bg-libr-accent2/20 rounded-2xl flex items-center justify-center mx-auto mb-4">
                      <AlertTriangle className="w-8 h-8 text-libr-accent2" />
                    </div>
                    <h3 className="text-lg font-medium text-foreground mb-2">No reports found</h3>
                    <p className="text-muted-foreground">No message reports available</p>
                  </motion.div>
                ) : (
                  <div className="grid gap-3">
                    {reports.map((report, index) => {
                      const { title, body } = parseContent(report.content);
                      const { textHtml, imgSrcs } = extractImages(body);
                      return (
                        <motion.div
                          key={report.sign}
                          initial={{ opacity: 0, y: 20 }}
                          animate={{ opacity: 1, y: 0 }}
                          transition={{ delay: index * 0.03 }}
                          className="bg-muted/30 border border-border/40 rounded-xl space-y-4 p-4"
                        >
                          <div className="mt-3 p-3 rounded-xl">
                            {/* Title */}
                            {title && (
                              <div className="text-sm font-semibold text-foreground mb-1">{title}</div>
                            )}

                            {/* Text body */}
                            <div className="text-sm text-foreground leading-snug">
                              <span className="font-semibold text-libr-secondary">Message: </span>
                              <span
                                className="leading-relaxed whitespace-pre-wrap break-words"
                                dangerouslySetInnerHTML={{ __html: textHtml }}
                              />
                            </div>

                            {/* Image gallery */}
                            {imgSrcs.length > 0 && (
                              <div className="mt-3 flex flex-wrap gap-2">
                                {imgSrcs.map((src, i) => (
                                  <div key={i} className="relative group cursor-pointer" onClick={() => setLightboxSrc(src)}>
                                    <img
                                      src={src}
                                      alt={`attachment-${i + 1}`}
                                      className="max-h-48 max-w-xs rounded-2xl object-cover border border-border/40 shadow-md group-hover:opacity-90 transition-opacity"
                                    />
                                    <div className="absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
                                      <div className="bg-black/50 rounded-full p-2">
                                        <ImageIcon className="w-4 h-4 text-white" />
                                      </div>
                                    </div>
                                  </div>
                                ))}
                              </div>
                            )}

                            {/* Reason */}
                            <div className="text-sm text-foreground leading-snug whitespace-pre-wrap mt-2">
                              <span className="font-semibold text-libr-secondary">Reason:</span>
                              <span className="ml-2">{report.note || "—"}</span>
                            </div>
                          </div>

                          <div className="flex items-center justify-end gap-2">
                            <motion.button
                              whileHover={{ scale: 1.05 }} whileTap={{ scale: 0.95 }}
                              onClick={() => handleModerate(report.sign, 'approve')}
                              className="libr-button bg-green-500 hover:bg-green-600 text-white flex items-center space-x-1"
                            >
                              <Check className="w-4 h-4" /><span>Approve</span>
                            </motion.button>
                            <motion.button
                              whileHover={{ scale: 1.05 }} whileTap={{ scale: 0.95 }}
                              onClick={() => handleModerate(report.sign, 'reject')}
                              className="libr-button bg-red-500 hover:bg-red-600 text-white flex items-center space-x-1"
                            >
                              <X className="w-4 h-4" /><span>Reject</span>
                            </motion.button>
                          </div>
                        </motion.div>
                      );
                    })}
                  </div>
                )}
              </motion.div>
            </div>
          </div>
        </div>
      </div>

      {/* Lightbox */}
      <AnimatePresence>
        {lightboxSrc && (
          <motion.div
            initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 backdrop-blur-sm"
            onClick={() => setLightboxSrc(null)}
          >
            <motion.div
              initial={{ scale: 0.85, opacity: 0 }} animate={{ scale: 1, opacity: 1 }} exit={{ scale: 0.85, opacity: 0 }}
              transition={{ type: 'spring', stiffness: 300, damping: 25 }}
              className="relative max-w-[90vw] max-h-[90vh]"
              onClick={e => e.stopPropagation()}
            >
              <img src={lightboxSrc} alt="Full size" className="max-w-[90vw] max-h-[90vh] rounded-2xl shadow-2xl object-contain" />
              <button
                onClick={() => setLightboxSrc(null)}
                className="absolute -top-3 -right-3 bg-card border border-border rounded-full p-1 shadow-lg hover:bg-muted transition"
              >
                <X className="w-4 h-4" />
              </button>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  );
};
